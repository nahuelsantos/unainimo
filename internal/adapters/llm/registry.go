package llm

import (
	"github.com/unanimo-ai/unanimo/internal/domain"
	"github.com/unanimo-ai/unanimo/internal/ports"
)

// MapRegistry wraps a fixed map of model id → player (cloud backends).
type MapRegistry struct {
	m map[domain.ModelID]ports.LLMPlayer
}

// NewMapRegistry builds a registry from a static map.
func NewMapRegistry(m map[domain.ModelID]ports.LLMPlayer) *MapRegistry {
	return &MapRegistry{m: m}
}

// Player returns a configured cloud adapter.
func (r *MapRegistry) Player(id domain.ModelID) (ports.LLMPlayer, bool) {
	if r == nil || r.m == nil {
		return nil, false
	}
	p, ok := r.m[id]
	return p, ok
}

// OllamaRegistry resolves any non-empty model name to an Ollama-backed player.
type OllamaRegistry struct {
	BaseURL string
}

// NewOllamaRegistry creates a dynamic registry for local Ollama tags.
func NewOllamaRegistry(baseURL string) *OllamaRegistry {
	return &OllamaRegistry{BaseURL: baseURL}
}

// Player returns an adapter for the given Ollama model tag.
func (r *OllamaRegistry) Player(id domain.ModelID) (ports.LLMPlayer, bool) {
	if r == nil || r.BaseURL == "" {
		return nil, false
	}
	s := string(id)
	if s == "" {
		return nil, false
	}
	return NewOllamaAdapter(r.BaseURL, s, id, s), true
}
