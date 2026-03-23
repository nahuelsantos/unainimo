package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	httphandler "github.com/unanimo-ai/unanimo/internal/adapters/http"
	"github.com/unanimo-ai/unanimo/internal/adapters/llm"
	"github.com/unanimo-ai/unanimo/internal/adapters/storage"
	"github.com/unanimo-ai/unanimo/internal/domain"
	"github.com/unanimo-ai/unanimo/internal/ports"
	"github.com/unanimo-ai/unanimo/internal/service"
)

func main() {
	// ── Config from environment ──────────────────────────────────────────────
	openaiKey := os.Getenv("OPENAI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	geminiKey := os.Getenv("GEMINI_API_KEY")
	groqKey := os.Getenv("GROQ_API_KEY")
	ollamaBase := resolveOllamaBase()

	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	redisPass := os.Getenv("REDIS_PASSWORD")
	port := getEnv("PORT", "8080")
	templatesDir := getEnv("TEMPLATES_DIR", "./web/templates")
	staticDir := getEnv("STATIC_DIR", "./web/static")

	// ── LLM registry (dynamic Ollama tags vs static cloud map) ───────────────
	players := make(map[domain.ModelID]ports.LLMPlayer)

	var ollamaJudge *llm.OllamaAdapter
	var registry ports.LLMPlayerRegistry

	if ollamaBase != "" {
		// Fallback tags must exist on your Ollama host (override with OLLAMA_* env vars).
		defaultModel := getEnv("OLLAMA_DEFAULT_MODEL", "llama3.1:8b")
		judgeModel := getEnv("OLLAMA_JUDGE_MODEL", defaultModel)
		ollamaJudge = llm.NewOllamaAdapter(ollamaBase, judgeModel, domain.ModelGPT4o, fmt.Sprintf("Judge · %s", judgeModel))
		registry = llm.NewOllamaRegistry(ollamaBase)
		log.Printf("✓ Ollama registry at %s (judge model %s)", ollamaBase, judgeModel)
	} else {
		if openaiKey != "" {
			players[domain.ModelGPT4o] = llm.NewOpenAIAdapter(openaiKey)
			log.Println("✓ OpenAI (GPT-4o) configured")
		} else {
			log.Println("✗ OpenAI key missing — GPT-4o disabled")
		}

		if anthropicKey != "" {
			players[domain.ModelClaude] = llm.NewAnthropicAdapter(anthropicKey)
			log.Println("✓ Anthropic (Claude 3.5) configured")
		} else {
			log.Println("✗ Anthropic key missing — Claude 3.5 disabled")
		}

		if geminiKey != "" {
			players[domain.ModelGemini] = llm.NewGeminiAdapter(geminiKey)
			log.Println("✓ Google (Gemini 1.5) configured")
		} else {
			log.Println("✗ Gemini key missing — Gemini 1.5 disabled")
		}

		if groqKey != "" {
			players[domain.ModelLlama] = llm.NewGroqAdapter(groqKey)
			log.Println("✓ Groq (Llama 3.1) configured")
		} else {
			log.Println("✗ Groq key missing — Llama 3.1 disabled")
		}

		log.Println("✗ Custom slot: no cloud adapter (use OLLAMA_BASE_URL for a local custom model)")
		registry = llm.NewMapRegistry(players)
	}

	if len(players) == 0 && ollamaBase == "" {
		log.Fatal("No LLM backends configured. Set OLLAMA_BASE_URL for local Ollama, or at least one of: OPENAI_API_KEY, ANTHROPIC_API_KEY, GEMINI_API_KEY, GROQ_API_KEY")
	}

	// ── Judge ─────────────────────────────────────────────────────────────────
	judge := llm.NewJudgeAdapter(openaiKey, anthropicKey, ollamaJudge)

	// ── Storage ───────────────────────────────────────────────────────────────
	var repo ports.GameRepository
	redisRepo, err := storage.NewRedisGameRepository(redisAddr, redisPass, 0)
	if err != nil {
		log.Printf("Redis unavailable (%v) — using in-memory storage", err)
		repo = storage.NewInMemoryGameRepository()
	} else {
		repo = redisRepo
		log.Printf("✓ Redis connected at %s", redisAddr)
	}

	// ── Event Emitter ─────────────────────────────────────────────────────────
	emitter := httphandler.NewInMemoryEventEmitter()

	// ── Service ───────────────────────────────────────────────────────────────
	// Single-GPU Ollama: run players serially by default (parallel + queue → client timeouts).
	serialOllama := ollamaBase != "" && ollamaSerialPlayersEnv()
	svc := service.NewGameService(registry, judge, repo, emitter, service.Config{SerialOllamaPlayers: serialOllama})
	if ollamaBase != "" {
		mode := "serial (one GPU / avoids queue timeouts)"
		if !serialOllama {
			mode = "parallel"
		}
		log.Printf("✓ Ollama player mode: %s", mode)
	}

	// ── HTTP Handler ──────────────────────────────────────────────────────────
	handler, err := httphandler.NewHandler(svc, emitter, templatesDir, httphandler.HandlerConfig{
		OllamaBaseURL:       ollamaBase,
		SerialOllamaPlayers: serialOllama,
	})
	if err != nil {
		log.Fatalf("create handler: %v", err)
	}

	mux := http.NewServeMux()
	httphandler.SetupRoutes(mux, handler, staticDir)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 30 * time.Minute, // SSE + long Ollama judge
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("🎮 UNANIMO AI starting on http://localhost:%s", port)

	// Graceful shutdown context (optional enhancement)
	ctx := context.Background()
	_ = ctx

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// resolveOllamaBase returns the Ollama HTTP API root.
// If OLLAMA_BASE_URL is unset, defaults to the lab IP (hostname plunder was unreliable for some routes).
// When the app runs like our Compose stack (/.dockerenv or REDIS_ADDR=redis:6379), loopback Ollama
// URLs are rewritten to host.docker.internal so they reach the Docker host, not the app container.
// Set OLLAMA_BASE_URL=- or "none" to disable Ollama and use cloud API keys only.
// ollamaSerialPlayersEnv: empty → serial (default). Set OLLAMA_SERIAL_PLAYERS=false for parallel cloud-style fan-out.
func ollamaSerialPlayersEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OLLAMA_SERIAL_PLAYERS")))
	if v == "" {
		return true
	}
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

func resolveOllamaBase() string {
	v := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL"))
	if v == "" {
		v = "http://192.168.1.130:11434"
	}
	if v == "-" || strings.EqualFold(v, "none") {
		return ""
	}
	rewritten := rewriteOllamaLocalhostForDocker(v)
	if rewritten != v {
		log.Printf("OLLAMA_BASE_URL adjusted for Docker: %q → %q (127.0.0.1/localhost is the container, not the host)", v, rewritten)
	}
	return rewritten
}

// rewriteOllamaLocalhostForDocker maps loopback to host.docker.internal when we are almost certainly
// inside a container talking to Compose Redis. Some runtimes omit /.dockerenv (e.g. rootless Podman).
func rewriteOllamaLocalhostForDocker(raw string) string {
	if !ollamaLoopbackRewriteLikelyInsideContainer() {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	h := strings.ToLower(u.Hostname())
	if h != "127.0.0.1" && h != "localhost" {
		return raw
	}
	port := u.Port()
	if port == "" {
		port = "11434"
	}
	u.Host = net.JoinHostPort("host.docker.internal", port)
	return u.String()
}

func ollamaLoopbackRewriteLikelyInsideContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// docker-compose.yml sets this for unanimo-app; host dev uses localhost:6379 or another host.
	return strings.TrimSpace(os.Getenv("REDIS_ADDR")) == "redis:6379"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, fmt.Sprintf("%.2fms", float64(time.Since(start).Microseconds())/1000))
	})
}
