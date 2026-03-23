package httphandler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unanimo-ai/unanimo/internal/adapters/llm"
	"github.com/unanimo-ai/unanimo/internal/domain"
	"github.com/unanimo-ai/unanimo/internal/service"
)

// HandlerConfig optional wiring (Ollama discovery URL, generation mode for UI).
type HandlerConfig struct {
	OllamaBaseURL        string
	SerialOllamaPlayers  bool
}

// Handler holds all HTTP handlers
type Handler struct {
	svc                   *service.GameService
	emitter               *InMemoryEventEmitter
	templates             *template.Template
	ollamaBase            string
	serialOllamaPlayers   bool
	httpClient            *http.Client
}

// NewHandler creates a new Handler
func NewHandler(svc *service.GameService, emitter *InMemoryEventEmitter, templatesDir string, cfg HandlerConfig) (*Handler, error) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Handler{
		svc:                  svc,
		emitter:              emitter,
		templates:            tmpl,
		ollamaBase:           strings.TrimSpace(cfg.OllamaBaseURL),
		serialOllamaPlayers:  cfg.SerialOllamaPlayers,
		httpClient:           &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"modelColor": func(id string) string {
			return domain.ColorForModel(domain.ModelID(id))
		},
		"modelColorMap": func() map[string]string {
			m := make(map[string]string)
			for k, v := range domain.ModelColors {
				m[string(k)] = v
			}
			return m
		},
		"modelName": func(id string) string {
			if n, ok := domain.ModelDisplayNames[domain.ModelID(id)]; ok {
				return n
			}
			return id
		},
		"add": func(a, b int) int { return a + b },
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
		"truncate": func(s string) string {
			if len(s) > 40 {
				return s[:37] + "..."
			}
			return s
		},
		"not": func(b bool) bool { return !b },
		"json": func(v interface{}) (template.JS, error) {
			b, err := json.Marshal(v)
			return template.JS(b), err
		},
	}
}

func ollamaListErrorMessage(base string, err error) string {
	s := fmt.Sprintf("Could not list Ollama models at %s (ollama tags: %v). Check OLLAMA_BASE_URL and that Ollama is running.", base, err)
	u, perr := url.Parse(base)
	if perr != nil {
		return s
	}
	h := strings.ToLower(u.Hostname())
	if h != "127.0.0.1" && h != "localhost" {
		return s
	}
	s += " Loopback only reaches Ollama on the same machine as this process. With Docker Compose, 127.0.0.1 is the app container, not your Mac: set OLLAMA_BASE_URL to http://host.docker.internal:11434 if Ollama runs on the Docker host, or use your LAN IP (e.g. http://192.168.1.130:11434) if Ollama is on another machine. Rebuild the app image after upgrading (make start / docker compose build)."
	return s
}

// IndexHandler serves the main page with concept selection
func (h *Handler) IndexHandler(w http.ResponseWriter, r *http.Request) {
	concepts := domain.GetRandomConcepts(10)
	data := map[string]interface{}{
		"Concepts":   concepts,
		"ConfigMode": "cloud",
		"Players":    domain.DefaultPlayerConfigs(),
	}

	if h.ollamaBase != "" {
		data["ConfigMode"] = "ollama"
		tags, err := llm.ListModelNames(r.Context(), h.ollamaBase, h.httpClient)
		if err != nil {
			log.Printf("ollama list tags: %v", err)
			data["OllamaListError"] = ollamaListErrorMessage(h.ollamaBase, err)
			data["Players"] = []domain.PlayerConfig{}
		} else {
			var cfgs []domain.PlayerConfig
			for _, name := range tags {
				cfgs = append(cfgs, domain.PlayerConfig{
					ModelID: domain.ModelID(name),
					Name:    name,
					Enabled: true,
					FormKey: domain.FormKeyForModel(name),
				})
			}
			data["Players"] = cfgs
		}
	}

	h.renderTemplate(w, "index.html", data)
}

// OllamaModelsHandler returns JSON model names from Ollama (GET /api/tags).
func (h *Handler) OllamaModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.ollamaBase == "" {
		http.Error(w, "ollama not configured", http.StatusNotFound)
		return
	}
	names, err := llm.ListModelNames(r.Context(), h.ollamaBase, h.httpClient)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(names)
}

// CreateGameHandler starts a new game
func (h *Handler) CreateGameHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	concept := r.FormValue("concept")
	if strings.TrimSpace(concept) == "" {
		http.Error(w, "concept is required", http.StatusBadRequest)
		return
	}

	configs, err := buildPlayerConfigs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	gameID := uuid.New().String()
	game := domain.NewGame(gameID, concept, configs)

	if err := h.svc.StartGame(r.Context(), game); err != nil {
		log.Printf("start game error: %v", err)
		http.Error(w, "failed to start game", http.StatusInternalServerError)
		return
	}

	// HTMX redirect to game page
	w.Header().Set("HX-Redirect", "/game/"+gameID)
	w.WriteHeader(http.StatusOK)
}

// GameHandler serves the game board page
func (h *Handler) GameHandler(w http.ResponseWriter, r *http.Request) {
	gameID := strings.TrimPrefix(r.URL.Path, "/game/")
	if gameID == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	game, err := h.svc.GetGame(r.Context(), gameID)
	if err != nil {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}

	order := domain.LegacyPlayerOrder(game)
	safeIDs := make(map[string]string)
	colors := make(map[string]string)
	for _, id := range order {
		mid := string(id)
		safeIDs[mid] = domain.SafeHTMLID(mid)
		colors[mid] = domain.ColorForModel(id)
	}

	playersDone := make(map[string]bool)
	for _, id := range order {
		if p, ok := game.Players[id]; ok && p.Done {
			playersDone[string(id)] = true
		}
	}

	data := map[string]interface{}{
		"Game":        game,
		"ModelOrder":  order,
		"SafeIDs":     safeIDs,
		"ModelColors": colors,
		"GameID":      gameID,
		"SerialOllamaPlayers": h.serialOllamaPlayers,
		"PlayersDone":         playersDone,
	}
	h.renderTemplate(w, "game.html", data)
}

// SSEHandler handles Server-Sent Events for real-time updates
func (h *Handler) SSEHandler(w http.ResponseWriter, r *http.Request) {
	gameID := strings.TrimPrefix(r.URL.Path, "/api/game/")
	gameID = strings.TrimSuffix(gameID, "/stream")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch, subID := h.emitter.Subscribe(gameID)
	defer h.emitter.Unsubscribe(gameID, subID)

	// Send initial ping
	fmt.Fprintf(w, "event: ping\ndata: connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		}
	}
}

// GameStateHandler returns current game state as JSON
func (h *Handler) GameStateHandler(w http.ResponseWriter, r *http.Request) {
	gameID := strings.TrimPrefix(r.URL.Path, "/api/game/")
	gameID = strings.TrimSuffix(gameID, "/state")

	game, err := h.svc.GetGame(r.Context(), gameID)
	if err != nil {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(game)
}

// ConceptsHandler returns new random concepts as HTMX partial
func (h *Handler) ConceptsHandler(w http.ResponseWriter, r *http.Request) {
	concepts := domain.GetRandomConcepts(10)
	data := map[string]interface{}{"Concepts": concepts}
	h.renderTemplate(w, "concepts_partial.html", data)
}

func (h *Handler) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s error: %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func buildPlayerConfigs(r *http.Request) ([]domain.PlayerConfig, error) {
	if r.FormValue("config_mode") == "ollama" {
		return buildOllamaPlayerConfigs(r)
	}
	return buildCloudPlayerConfigs(r), nil
}

func buildOllamaPlayerConfigs(r *http.Request) ([]domain.PlayerConfig, error) {
	selected := r.Form["ollama_model"]
	var configs []domain.PlayerConfig
	seen := map[string]bool{}
	for _, name := range selected {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		pk := domain.FormKeyForModel(name)
		persona := r.FormValue("persona_" + pk)
		configs = append(configs, domain.PlayerConfig{
			ModelID: domain.ModelID(name),
			Name:    name,
			Enabled: true,
			Persona: persona,
			FormKey: pk,
		})
	}
	if len(configs) < 2 {
		return nil, fmt.Errorf("select at least two Ollama models")
	}
	return configs, nil
}

func buildCloudPlayerConfigs(r *http.Request) []domain.PlayerConfig {
	defaults := domain.DefaultPlayerConfigs()
	var configs []domain.PlayerConfig

	for _, def := range defaults {
		cfg := def
		enabledKey := fmt.Sprintf("player_%s_enabled", def.ModelID)
		personaKey := fmt.Sprintf("player_%s_persona", def.ModelID)

		cfg.Enabled = r.FormValue(enabledKey) == "on" || r.FormValue(enabledKey) == "true"
		cfg.Persona = r.FormValue(personaKey)

		if def.ModelID == domain.ModelCustom {
			if name := r.FormValue("custom_name"); name != "" {
				cfg.Name = name
			}
		}

		configs = append(configs, cfg)
	}

	enabledCount := 0
	for _, c := range configs {
		if c.Enabled {
			enabledCount++
		}
	}
	if enabledCount < 2 {
		for i := range configs {
			if i < 2 {
				configs[i].Enabled = true
			}
		}
	}

	return configs
}
