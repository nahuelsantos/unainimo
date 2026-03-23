package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/unanimo-ai/unanimo/internal/domain"
)

// OllamaAdapter talks to an Ollama server using POST /api/chat (native API),
// or OpenAI-compatible POST /v1/chat/completions when OLLAMA_OPENAI_COMPAT is set.
type OllamaAdapter struct {
	baseURL          string
	ollamaModel      string
	modelID          domain.ModelID
	name             string
	useOpenAICompat  bool
	openAICompatAuth string // optional Bearer for gateways (OLLAMA_OPENAI_COMPAT_KEY)
	client           *http.Client
}

// NewOllamaAdapter creates a player backed by a specific Ollama model.
// baseURL is the HTTP root of the Ollama API (e.g. http://plunder:11434), not the web UI unless it proxies /api or /v1.
func NewOllamaAdapter(baseURL, ollamaModel string, modelID domain.ModelID, displayName string) *OllamaAdapter {
	compat := envTruthy("OLLAMA_OPENAI_COMPAT")
	auth := strings.TrimSpace(os.Getenv("OLLAMA_OPENAI_COMPAT_KEY"))
	return &OllamaAdapter{
		baseURL:          strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		ollamaModel:      strings.TrimSpace(ollamaModel),
		modelID:          modelID,
		name:             displayName,
		useOpenAICompat:  compat,
		openAICompatAuth: auth,
		client: &http.Client{
			Timeout: ollamaHTTPTimeout(),
		},
	}
}

// ollamaHTTPTimeout is per-request time waiting for Ollama (large models + GPU queue).
// Default 15m; set OLLAMA_HTTP_TIMEOUT to seconds (e.g. 1800 for 30m).
func ollamaHTTPTimeout() time.Duration {
	v := strings.TrimSpace(os.Getenv("OLLAMA_HTTP_TIMEOUT"))
	if v == "" {
		return 15 * time.Minute
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(sec) * time.Second
}

func envTruthy(k string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes"
}

func (a *OllamaAdapter) GetModelID() domain.ModelID { return a.modelID }
func (a *OllamaAdapter) GetName() string            { return a.name }

// GenerateWords calls Ollama to produce 8 associated words.
func (a *OllamaAdapter) GenerateWords(ctx context.Context, concept string, persona string) ([]string, error) {
	systemPrompt := buildSystemPrompt(persona)
	userPrompt := buildWordPrompt(concept)
	raw, err := a.callChat(ctx, systemPrompt, userPrompt, 0.7)
	if err != nil {
		return nil, err
	}
	return parseWordsFromJSON(raw)
}

// ClusterWords implements semantic clustering via the same Ollama model (judge role).
func (a *OllamaAdapter) ClusterWords(ctx context.Context, words []string, concept string) (map[string]int, error) {
	system := judgeSystemPrompt()
	user := buildJudgePrompt(words, concept)
	raw, err := a.callChat(ctx, system, user, 0.1)
	if err != nil {
		return nil, err
	}
	return parseClusterResponse(raw)
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error,omitempty"`
}

func (a *OllamaAdapter) callChat(ctx context.Context, system, user string, temp float64) (string, error) {
	if a.baseURL == "" {
		return "", fmt.Errorf("ollama: empty base URL")
	}
	if a.ollamaModel == "" {
		return "", fmt.Errorf("ollama: empty model name")
	}
	if a.useOpenAICompat {
		return a.callOpenAICompat(ctx, system, user, temp)
	}

	reqBody := ollamaChatRequest{
		Model: a.ollamaModel,
		Messages: []ollamaMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Stream: false,
		Options: ollamaOptions{
			Temperature: temp,
			NumPredict:  512,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama marshal: %w", err)
	}

	url := a.baseURL + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var apiResp ollamaChatResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("ollama parse json: %w (body: %s)", err, truncateForErr(body))
	}
	if apiResp.Error != "" {
		return "", fmt.Errorf("ollama: %s", apiResp.Error)
	}

	return strings.TrimSpace(apiResp.Message.Content), nil
}

// callOpenAICompat uses Ollama's OpenAI-compatible API (/v1/chat/completions).
func (a *OllamaAdapter) callOpenAICompat(ctx context.Context, system, user string, temp float64) (string, error) {
	reqBody := openAIRequest{
		Model: a.ollamaModel,
		Messages: []openAIMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: temp,
		MaxTokens:   512,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama openai-compat marshal: %w", err)
	}

	url := a.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama openai-compat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.openAICompatAuth != "" {
		req.Header.Set("Authorization", "Bearer "+a.openAICompatAuth)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama openai-compat http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama openai-compat read: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama openai-compat: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("ollama openai-compat parse: %w", err)
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("ollama openai-compat: %s", apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("ollama openai-compat: no choices")
	}
	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
}

func truncateForErr(b []byte) string {
	s := string(b)
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}
