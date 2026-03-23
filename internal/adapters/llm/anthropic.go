package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/unanimo-ai/unanimo/internal/domain"
)

// AnthropicAdapter implements LLMPlayer using the Anthropic API
type AnthropicAdapter struct {
	apiKey  string
	model   string
	name    string
	modelID domain.ModelID
}

// NewAnthropicAdapter creates a new Anthropic adapter
func NewAnthropicAdapter(apiKey string) *AnthropicAdapter {
	return &AnthropicAdapter{
		apiKey:  apiKey,
		model:   "claude-sonnet-4-5",
		name:    "Claude 3.5",
		modelID: domain.ModelClaude,
	}
}

func (a *AnthropicAdapter) GetModelID() domain.ModelID { return a.modelID }
func (a *AnthropicAdapter) GetName() string            { return a.name }

// GenerateWords calls the Anthropic API to produce 8 associated words
func (a *AnthropicAdapter) GenerateWords(ctx context.Context, concept string, persona string) ([]string, error) {
	systemPrompt := buildSystemPrompt(persona)
	userPrompt := buildWordPrompt(concept)
	raw, err := a.callAPI(ctx, systemPrompt, userPrompt, 0.7)
	if err != nil {
		return nil, err
	}
	return parseWordsFromJSON(raw)
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *AnthropicAdapter) callAPI(ctx context.Context, system, user string, temp float64) (string, error) {
	reqBody := anthropicRequest{
		Model:     a.model,
		MaxTokens: 300,
		System:    system,
		Messages: []anthropicMessage{
			{Role: "user", Content: user},
		},
		Temperature: temp,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	for _, block := range apiResp.Content {
		if block.Type == "text" {
			return strings.TrimSpace(block.Text), nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}
