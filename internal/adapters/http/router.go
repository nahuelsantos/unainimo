package httphandler

import (
	"net/http"
)

// SetupRoutes configures all HTTP routes
func SetupRoutes(mux *http.ServeMux, h *Handler, staticDir string) {
	// Static files (PWA assets, icons, etc.)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// PWA files served at root
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		http.ServeFile(w, r, staticDir+"/manifest.json")
	})
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, staticDir+"/sw.js")
	})

	// API — Ollama discovery (JSON)
	mux.HandleFunc("/api/ollama/models", h.OllamaModelsHandler)

	// Pages
	mux.HandleFunc("/", h.IndexHandler)
	mux.HandleFunc("/game/", h.GameHandler)

	// API endpoints
	mux.HandleFunc("/api/game", h.CreateGameHandler)
	mux.HandleFunc("/api/game/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case len(path) > 5 && path[len(path)-7:] == "/stream":
			h.SSEHandler(w, r)
		case len(path) > 5 && path[len(path)-6:] == "/state":
			h.GameStateHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	// Partial templates
	mux.HandleFunc("/api/concepts", h.ConceptsHandler)
}
