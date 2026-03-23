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

// OpenAIAdapter implements LLMPlayer and Judge using the OpenAI API
type OpenAIAdapter struct {
	apiKey  string
	model   string
	baseURL string
	name    string
	modelID domain.ModelID
}

// NewOpenAIAdapter creates a new OpenAI adapter
func NewOpenAIAdapter(apiKey string) *OpenAIAdapter {
	return &OpenAIAdapter{
		apiKey:  apiKey,
		model:   "gpt-4o",
		baseURL: "https://api.openai.com/v1",
		name:    "GPT-4o",
		modelID: domain.ModelGPT4o,
	}
}

func (a *OpenAIAdapter) GetModelID() domain.ModelID { return a.modelID }
func (a *OpenAIAdapter) GetName() string            { return a.name }

// GenerateWords calls the OpenAI API to produce 8 associated words
func (a *OpenAIAdapter) GenerateWords(ctx context.Context, concept string, persona string) ([]string, error) {
	systemPrompt := buildSystemPrompt(persona)
	userPrompt := buildWordPrompt(concept)

	return a.callChatAPI(ctx, systemPrompt, userPrompt, 0.7)
}

// ClusterWords calls the OpenAI API to cluster words semantically
func (a *OpenAIAdapter) ClusterWords(ctx context.Context, words []string, concept string) (map[string]int, error) {
	systemPrompt := judgeSystemPrompt()
	userPrompt := buildJudgePrompt(words, concept)

	response, err := a.callChatAPIRaw(ctx, systemPrompt, userPrompt, 0.1)
	if err != nil {
		return nil, err
	}
	return parseClusterResponse(response)
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *OpenAIAdapter) callChatAPI(ctx context.Context, system, user string, temp float64) ([]string, error) {
	raw, err := a.callChatAPIRaw(ctx, system, user, temp)
	if err != nil {
		return nil, err
	}
	return parseWordsFromJSON(raw)
}

func (a *OpenAIAdapter) callChatAPIRaw(ctx context.Context, system, user string, temp float64) (string, error) {
	reqBody := openAIRequest{
		Model: a.model,
		Messages: []openAIMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: temp,
		MaxTokens:   300,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
}
