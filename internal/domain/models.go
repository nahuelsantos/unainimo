package domain

import "time"

// ModelID identifies an LLM provider
type ModelID string

const (
	ModelGPT4o   ModelID = "gpt-4o"
	ModelClaude  ModelID = "claude-sonnet-4-5"
	ModelGemini  ModelID = "gemini-1.5-pro"
	ModelLlama   ModelID = "llama-3.1-70b-versatile"
	ModelCustom  ModelID = "custom"
)

// ModelOrder defines display order for consistent column layout
var ModelOrder = []ModelID{ModelGPT4o, ModelClaude, ModelGemini, ModelLlama, ModelCustom}

// ModelDisplayNames maps model IDs to human-readable names
var ModelDisplayNames = map[ModelID]string{
	ModelGPT4o:  "GPT-4o",
	ModelClaude: "Claude 3.5",
	ModelGemini: "Gemini 1.5",
	ModelLlama:  "Llama 3.1",
	ModelCustom: "Custom",
}

// ModelColors for UI differentiation
var ModelColors = map[ModelID]string{
	ModelGPT4o:  "#10a37f", // OpenAI green
	ModelClaude: "#d97757", // Anthropic orange
	ModelGemini: "#4285f4", // Google blue
	ModelLlama:  "#7c3aed", // Meta purple
	ModelCustom: "#6b7280", // Gray
}

// GameStatus tracks game lifecycle
type GameStatus string

const (
	StatusWaiting  GameStatus = "waiting"
	StatusRunning  GameStatus = "running"
	StatusJudging  GameStatus = "judging"
	StatusComplete GameStatus = "complete"
	StatusError    GameStatus = "error"
)

// Word represents a single generated word with its metadata
type Word struct {
	Text      string `json:"text"`
	Position  int    `json:"position"`  // 1-8
	ClusterID int    `json:"cluster_id"` // 0 = unassigned
}

// PlayerConfig holds configuration for an LLM player
type PlayerConfig struct {
	ModelID  ModelID `json:"model_id"`
	Name     string  `json:"name"`
	Persona  string  `json:"persona"`
	Enabled  bool    `json:"enabled"`
	APIKey   string  `json:"api_key,omitempty"` // for custom model overrides
	// FormKey is a stable form field suffix for persona (avoids ':' in HTML ids); set server-side for Ollama.
	FormKey string `json:"form_key,omitempty"`
}

// PlayerResult holds the outcome for one LLM player
type PlayerResult struct {
	Config             PlayerConfig `json:"config"`
	Words              []Word       `json:"words"`
	UnanimoScore       int          `json:"unanimo_score"`
	SynchronicityScore int          `json:"synchronicity_score"`
	Bonus              int          `json:"bonus"`
	TotalUnanimoScore  int          `json:"total_unanimo_score"`
	Error              string       `json:"error,omitempty"`
	Done               bool         `json:"done"`
}

// Game is the central aggregate root
type Game struct {
	ID          string                    `json:"id"`
	Concept     string                    `json:"concept"`
	Players     map[ModelID]*PlayerResult `json:"players"`
	PlayerOrder []ModelID                 `json:"player_order,omitempty"` // column / SSE order
	Status      GameStatus                `json:"status"`
	Clusters    map[string]int            `json:"clusters,omitempty"` // word -> cluster_id
	CreatedAt   time.Time                 `json:"created_at"`
}

// NewGame creates a new game with the given concept and player configs
func NewGame(id, concept string, configs []PlayerConfig) *Game {
	players := make(map[ModelID]*PlayerResult)
	var order []ModelID
	for _, cfg := range configs {
		if cfg.Enabled {
			order = append(order, cfg.ModelID)
			players[cfg.ModelID] = &PlayerResult{
				Config: cfg,
				Words:  []Word{},
			}
		}
	}
	return &Game{
		ID:          id,
		Concept:     concept,
		Players:     players,
		PlayerOrder: order,
		Status:      StatusWaiting,
		CreatedAt:   time.Now(),
	}
}

// AllWordsCollected returns true if all enabled players have submitted 8 words or errored
func (g *Game) AllWordsCollected() bool {
	for _, p := range g.Players {
		if !p.Done {
			return false
		}
	}
	return true
}

// GetAllWords returns all words from all players as a flat slice
func (g *Game) GetAllWords() []string {
	seen := map[string]bool{}
	var words []string
	for _, p := range g.Players {
		for _, w := range p.Words {
			if !seen[w.Text] {
				seen[w.Text] = true
				words = append(words, w.Text)
			}
		}
	}
	return words
}

// ActivePlayerCount returns the number of enabled, non-errored players
func (g *Game) ActivePlayerCount() int {
	count := 0
	for _, p := range g.Players {
		if p.Error == "" {
			count++
		}
	}
	return count
}

// DefaultPlayerConfigs returns the default set of 5 LLM players
func DefaultPlayerConfigs() []PlayerConfig {
	cfgs := []PlayerConfig{
		{ModelID: ModelGPT4o, Name: "GPT-4o", Enabled: true},
		{ModelID: ModelClaude, Name: "Claude 3.5", Enabled: true},
		{ModelID: ModelGemini, Name: "Gemini 1.5", Enabled: true},
		{ModelID: ModelLlama, Name: "Llama 3.1", Enabled: true},
		{ModelID: ModelCustom, Name: "Custom", Enabled: false},
	}
	for i := range cfgs {
		cfgs[i].FormKey = FormKeyForModel(string(cfgs[i].ModelID))
	}
	return cfgs
}
