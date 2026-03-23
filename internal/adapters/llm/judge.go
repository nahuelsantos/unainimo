package llm

import (
	"context"
	"fmt"
)

// JudgeAdapter wraps an LLM to perform semantic clustering
// It defaults to OpenAI but can use any available model
type JudgeAdapter struct {
	ollamaJudge *OllamaAdapter
	openai      *OpenAIAdapter
	anthropic   *AnthropicAdapter
}

// NewJudgeAdapter creates a judge that uses the best available model.
// If ollamaJudge is non-nil, it is tried first (local lab).
func NewJudgeAdapter(openaiKey, anthropicKey string, ollamaJudge *OllamaAdapter) *JudgeAdapter {
	j := &JudgeAdapter{ollamaJudge: ollamaJudge}
	if openaiKey != "" {
		j.openai = NewOpenAIAdapter(openaiKey)
	}
	if anthropicKey != "" {
		j.anthropic = NewAnthropicAdapter(anthropicKey)
	}
	return j
}

// ClusterWords groups words into semantic clusters using an LLM
func (j *JudgeAdapter) ClusterWords(ctx context.Context, words []string, concept string) (map[string]int, error) {
	system := judgeSystemPrompt()
	prompt := buildJudgePrompt(words, concept)

	if j.ollamaJudge != nil {
		clusters, err := j.ollamaJudge.ClusterWords(ctx, words, concept)
		if err == nil {
			return clusters, nil
		}
	}

	// Prefer OpenAI for judging; fall back to Anthropic
	if j.openai != nil {
		raw, err := j.openai.callChatAPIRaw(ctx, system, prompt, 0.1)
		if err == nil {
			clusters, err := parseClusterResponse(raw)
			if err == nil {
				return clusters, nil
			}
		}
	}

	if j.anthropic != nil {
		raw, err := j.anthropic.callAPI(ctx, system, prompt, 0.1)
		if err == nil {
			return parseClusterResponse(raw)
		}
	}

	return nil, fmt.Errorf("judge unavailable: set OLLAMA_BASE_URL + OLLAMA_JUDGE_MODEL, or OPENAI_API_KEY, or ANTHROPIC_API_KEY")
}
