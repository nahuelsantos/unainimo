package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ollamaTagsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Details struct {
			Family   string   `json:"family"`
			Families []string `json:"families"`
		} `json:"details"`
	} `json:"models"`
}

// ListModelNames returns chat-capable model names from Ollama GET /api/tags.
// Embedding-only and other non-chat models are omitted so the UI does not offer them.
func ListModelNames(ctx context.Context, baseURL string, client *http.Client) ([]string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("ollama: empty base URL")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama tags: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama tags: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed ollamaTagsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("ollama tags parse: %w", err)
	}

	var names []string
	for _, m := range parsed.Models {
		n := strings.TrimSpace(m.Name)
		if n == "" || !modelSupportsChat(n, m.Details) {
			continue
		}
		names = append(names, n)
	}
	return names, nil
}

func modelSupportsChat(name string, details struct {
	Family   string   `json:"family"`
	Families []string `json:"families"`
}) bool {
	ln := strings.ToLower(name)
	if strings.Contains(ln, "embed") {
		return false
	}
	if strings.ToLower(details.Family) == "nomic-bert" {
		return false
	}
	for _, x := range details.Families {
		if strings.ToLower(x) == "nomic-bert" {
			return false
		}
	}
	return true
}
