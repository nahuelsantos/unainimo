package ports

import (
	"context"

	"github.com/unanimo-ai/unanimo/internal/domain"
)

// LLMPlayerRegistry resolves an LLM player by model id (cloud map or dynamic Ollama names).
type LLMPlayerRegistry interface {
	Player(modelID domain.ModelID) (LLMPlayer, bool)
}

// LLMPlayer is the port for an LLM that can generate words
type LLMPlayer interface {
	// GenerateWords produces exactly 8 words associated with the concept
	GenerateWords(ctx context.Context, concept string, persona string) ([]string, error)
	// GetModelID returns this player's model identifier
	GetModelID() domain.ModelID
	// GetName returns the display name
	GetName() string
}

// Judge is the port for the semantic clustering LLM
type Judge interface {
	// ClusterWords groups the given words into semantic clusters
	// Returns map: word -> cluster_id (integers, 1-based)
	ClusterWords(ctx context.Context, words []string, concept string) (map[string]int, error)
}

// GameRepository is the port for game persistence
type GameRepository interface {
	Save(ctx context.Context, game *domain.Game) error
	Get(ctx context.Context, gameID string) (*domain.Game, error)
	Delete(ctx context.Context, gameID string) error
}

// EventEmitter is the port for SSE event broadcasting
type EventEmitter interface {
	// Emit sends an event to all subscribers of a game
	Emit(gameID string, event Event)
	// Subscribe returns a channel that receives events for the game
	Subscribe(gameID string) (<-chan Event, string)
	// Unsubscribe removes a subscriber
	Unsubscribe(gameID string, subscriberID string)
}

// EventType defines the types of SSE events
type EventType string

const (
	EventWordAdded     EventType = "word-added"
	EventPlayerDone    EventType = "player-done"
	EventPlayerError   EventType = "player-error"
	EventJudgeStart    EventType = "judge-start"
	EventJudgeComplete EventType = "judge-complete"
	EventScoreUpdate   EventType = "score-update"
	EventGameComplete  EventType = "game-complete"
	EventGameError     EventType = "game-error"
)

// Event is a single SSE payload
type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
}

// WordAddedPayload is the payload for word-added events
type WordAddedPayload struct {
	ModelID  domain.ModelID `json:"model_id"`
	Word     domain.Word    `json:"word"`
}

// PlayerDonePayload is the payload for player-done events
type PlayerDonePayload struct {
	ModelID domain.ModelID `json:"model_id"`
	Words   []domain.Word  `json:"words"`
}

// ScoreUpdatePayload carries the scoring results
type ScoreUpdatePayload struct {
	Players  map[domain.ModelID]*domain.PlayerResult `json:"players"`
	Clusters map[string]int                          `json:"clusters"`
	Phase    string                                  `json:"phase"` // "strict" | "semantic"
}
