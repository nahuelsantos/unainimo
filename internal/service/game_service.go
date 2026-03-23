package service

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/unanimo-ai/unanimo/internal/domain"
	"github.com/unanimo-ai/unanimo/internal/ports"
)

// Config toggles orchestration behavior (e.g. single-GPU Ollama labs).
type Config struct {
	// SerialOllamaPlayers runs one player at a time when true, avoiding GPU queue + HTTP timeouts.
	SerialOllamaPlayers bool
}

// GameService orchestrates the full Unanimo game flow
type GameService struct {
	registry ports.LLMPlayerRegistry
	judge    ports.Judge
	repo     ports.GameRepository
	emitter  ports.EventEmitter
	cfg      Config
}

// NewGameService creates a new GameService with all dependencies
func NewGameService(
	registry ports.LLMPlayerRegistry,
	judge ports.Judge,
	repo ports.GameRepository,
	emitter ports.EventEmitter,
	cfg Config,
) *GameService {
	return &GameService{
		registry: registry,
		judge:    judge,
		repo:     repo,
		emitter:  emitter,
		cfg:      cfg,
	}
}

// StartGame initializes a new game and begins the LLM generation pipeline
func (s *GameService) StartGame(ctx context.Context, game *domain.Game) error {
	game.Status = domain.StatusRunning
	if err := s.repo.Save(ctx, game); err != nil {
		return fmt.Errorf("save game: %w", err)
	}

	// Run all LLM players concurrently in background
	go s.runGame(game)
	return nil
}

// GetGame retrieves the current state of a game
func (s *GameService) GetGame(ctx context.Context, gameID string) (*domain.Game, error) {
	return s.repo.Get(ctx, gameID)
}

// runGame is the main async game loop
func (s *GameService) runGame(game *domain.Game) {
	ctx := context.Background()

	if s.cfg.SerialOllamaPlayers {
		order := game.PlayerOrder
		if len(order) == 0 {
			for mid := range game.Players {
				order = append(order, mid)
			}
		}
		for _, mid := range order {
			if p, ok := game.Players[mid]; ok {
				s.runPlayer(ctx, game, mid, p)
			}
		}
	} else {
		var wg sync.WaitGroup
		for modelID, player := range game.Players {
			wg.Add(1)
			go func(mid domain.ModelID, p *domain.PlayerResult) {
				defer wg.Done()
				s.runPlayer(ctx, game, mid, p)
			}(modelID, player)
		}
		wg.Wait()
	}

	// All players done — reload game state and run judge
	game, err := s.repo.Get(ctx, game.ID)
	if err != nil {
		log.Printf("[game %s] failed to reload for judging: %v", game.ID, err)
		return
	}

	// Apply strict clustering first for immediate partial scores
	domain.ApplyStrictClusters(game)
	domain.CalculateScores(game)
	if err := s.repo.Save(ctx, game); err != nil {
		log.Printf("[game %s] save after strict: %v", game.ID, err)
	}

	s.emitter.Emit(game.ID, ports.Event{
		Type: ports.EventScoreUpdate,
		Payload: ports.ScoreUpdatePayload{
			Players:  game.Players,
			Clusters: game.Clusters,
			Phase:    "strict",
		},
	})

	// Run semantic judge
	game.Status = domain.StatusJudging
	s.repo.Save(ctx, game)
	s.emitter.Emit(game.ID, ports.Event{Type: ports.EventJudgeStart, Payload: nil})

	allWords := game.GetAllWords()
	if len(allWords) > 0 {
		clusters, err := s.judge.ClusterWords(ctx, allWords, game.Concept)
		if err != nil {
			log.Printf("[game %s] judge error: %v", game.ID, err)
			// Fall back to strict clusters
		} else {
			domain.ApplyClusters(game, clusters)
			domain.CalculateScores(game)

			s.emitter.Emit(game.ID, ports.Event{
				Type: ports.EventJudgeComplete,
				Payload: map[string]interface{}{
					"clusters": clusters,
				},
			})
		}
	}

	game.Status = domain.StatusComplete
	s.repo.Save(ctx, game)

	s.emitter.Emit(game.ID, ports.Event{
		Type: ports.EventGameComplete,
		Payload: ports.ScoreUpdatePayload{
			Players:  game.Players,
			Clusters: game.Clusters,
			Phase:    "semantic",
		},
	})
}

// runPlayer calls an LLM player and collects their words
func (s *GameService) runPlayer(ctx context.Context, game *domain.Game, modelID domain.ModelID, player *domain.PlayerResult) {
	llm, ok := s.registry.Player(modelID)
	if !ok {
		s.markPlayerError(ctx, game, modelID, "no adapter configured")
		return
	}

	words, err := llm.GenerateWords(ctx, game.Concept, player.Config.Persona)
	if err != nil {
		log.Printf("[game %s][%s] generation error: %v", game.ID, modelID, err)
		s.markPlayerError(ctx, game, modelID, err.Error())
		return
	}

	// Build word objects and emit SSE events
	var domainWords []domain.Word
	for i, w := range words {
		dw := domain.Word{Text: w, Position: i + 1}
		domainWords = append(domainWords, dw)
		s.emitter.Emit(game.ID, ports.Event{
			Type: ports.EventWordAdded,
			Payload: ports.WordAddedPayload{
				ModelID: modelID,
				Word:    dw,
			},
		})
	}

	// Update game state in Redis atomically
	s.updatePlayerWords(ctx, game.ID, modelID, domainWords)

	s.emitter.Emit(game.ID, ports.Event{
		Type: ports.EventPlayerDone,
		Payload: ports.PlayerDonePayload{
			ModelID: modelID,
			Words:   domainWords,
		},
	})
}

// updatePlayerWords updates a player's words in the stored game
func (s *GameService) updatePlayerWords(ctx context.Context, gameID string, modelID domain.ModelID, words []domain.Word) {
	game, err := s.repo.Get(ctx, gameID)
	if err != nil {
		log.Printf("[game %s] reload for word update: %v", gameID, err)
		return
	}
	if p, ok := game.Players[modelID]; ok {
		p.Words = words
		p.Done = true
		game.Players[modelID] = p
	}
	if err := s.repo.Save(ctx, game); err != nil {
		log.Printf("[game %s] save after words: %v", gameID, err)
	}
}

func (s *GameService) markPlayerError(ctx context.Context, game *domain.Game, modelID domain.ModelID, errMsg string) {
	g, err := s.repo.Get(ctx, game.ID)
	if err != nil {
		return
	}
	if p, ok := g.Players[modelID]; ok {
		p.Error = errMsg
		p.Done = true
		g.Players[modelID] = p
	}
	s.repo.Save(ctx, g)

	s.emitter.Emit(game.ID, ports.Event{
		Type:    ports.EventPlayerError,
		Payload: map[string]string{"model_id": string(modelID), "error": errMsg},
	})
}
