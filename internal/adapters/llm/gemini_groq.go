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

// ─── Gemini ──────────────────────────────────────────────────────────────────

// GeminiAdapter implements LLMPlayer using Google's Gemini API
type GeminiAdapter struct {
	apiKey  string
	model   string
	name    string
	modelID domain.ModelID
}

func NewGeminiAdapter(apiKey string) *GeminiAdapter {
	return &GeminiAdapter{
		apiKey:  apiKey,
		model:   "gemini-1.5-pro",
		name:    "Gemini 1.5",
		modelID: domain.ModelGemini,
	}
}

func (a *GeminiAdapter) GetModelID() domain.ModelID { return a.modelID }
func (a *GeminiAdapter) GetName() string            { return a.name }

func (a *GeminiAdapter) GenerateWords(ctx context.Context, concept string, persona string) ([]string, error) {
	fullPrompt := buildSystemPrompt(persona) + "\n\n" + buildWordPrompt(concept)

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		a.model, a.apiKey,
	)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": fullPrompt}}},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 300,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	raw := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	return parseWordsFromJSON(raw)
}

// ─── Groq (OpenAI-compatible) ─────────────────────────────────────────────────

// GroqAdapter implements LLMPlayer using Groq's OpenAI-compatible API
type GroqAdapter struct {
	apiKey  string
	model   string
	name    string
	modelID domain.ModelID
}

func NewGroqAdapter(apiKey string) *GroqAdapter {
	return &GroqAdapter{
		apiKey:  apiKey,
		model:   "llama-3.1-70b-versatile",
		name:    "Llama 3.1",
		modelID: domain.ModelLlama,
	}
}

func (a *GroqAdapter) GetModelID() domain.ModelID { return a.modelID }
func (a *GroqAdapter) GetName() string            { return a.name }

func (a *GroqAdapter) GenerateWords(ctx context.Context, concept string, persona string) ([]string, error) {
	systemPrompt := buildSystemPrompt(persona)
	userPrompt := buildWordPrompt(concept)

	reqBody := map[string]interface{}{
		"model": a.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.7,
		"max_tokens":  300,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return parseWordsFromJSON(strings.TrimSpace(apiResp.Choices[0].Message.Content))
}

// ─── Custom Persona Adapter ───────────────────────────────────────────────────

// CustomAdapter uses OpenAI API with a custom model + persona
type CustomAdapter struct {
	inner   *OpenAIAdapter
	name    string
	modelID domain.ModelID
}

func NewCustomAdapter(apiKey, model, name string) *CustomAdapter {
	inner := NewOpenAIAdapter(apiKey)
	inner.model = model
	inner.name = name
	inner.modelID = domain.ModelCustom
	return &CustomAdapter{
		inner:   inner,
		name:    name,
		modelID: domain.ModelCustom,
	}
}

func (a *CustomAdapter) GetModelID() domain.ModelID { return a.modelID }
func (a *CustomAdapter) GetName() string            { return a.name }

func (a *CustomAdapter) GenerateWords(ctx context.Context, concept string, persona string) ([]string, error) {
	return a.inner.GenerateWords(ctx, concept, persona)
}
