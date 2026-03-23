# 🧠 unAInimo: Technical Specification & Implementation Prompt

A Progressive Web App (PWA) that tests **conceptual and priority alignment** between AI models — a digital adaptation of the Unanimo board game using Hexagonal Architecture, Go, and HTMX.

## 1. Core Mechanics & Rules
* **The Round**: A user selects one of 10 random concepts.
* **The Players**: 5 LLM "Players" (GPT-4o, Claude 3.5 Sonnet, Gemini 1.5 Pro, Llama 3.1 70b via Groq, and a Custom Persona slot).
* **Generation**: Each model generates exactly **8 words** related to the concept simultaneously.
* **Forbidden Content**: Words sharing a root with the concept, numbers (1, 2, 3...), or symbols (-, =, +) are disqualified.
* **The Judge (Semantic Referee)**: Once all 40 words are collected, a "Judge" LLM (e.g., GPT-4o-mini) groups them into semantic clusters. 
    * *Judge Prompt:* "Group these 40 words into semantic clusters. Words with the same meaning or root (e.g., 'Newspaper'/'Newspapers' or 'Fast'/'Quick') must share a unique `cluster_id`. Return a JSON map: `{"word": cluster_id}`."

## 2. Scoring Engine Logic
The engine calculates two distinct scores per model based on the Judge's clusters:

### A. Unanimo Score (Conceptual Alignment)
For each word in a model's list:
1. Identify the `cluster_id` assigned by the Judge.
2. `cluster_size` = total number of models that have at least one word in that `cluster_id`.
3. If `cluster_size >= 2`, the model gains points equal to `cluster_size`. Otherwise, 0.
4. **+5 Bonus**: If the total Unanimo score $\ge 18$ (the 5-player threshold), add a 5-point bonus.

### B. Synchronicity Score (Priority Alignment)
This measures if models rank the same concepts at the same level of importance.
For each word at position $P$ (1-8) in cluster $C$:
1. $M$ = number of models that have a word from cluster $C$ at the **exact same position $P$**.
2. If $M \ge 2$, `sync_score += (unanimo_base_points * M)`.
3. If $M < 2$, `sync_score += unanimo_base_points`.

## 3. Technical Stack & Architecture
* **Backend**: Golang 1.23+ (Hexagonal Architecture).
* **Frontend**: HTML5, Tailwind CSS, **HTMX**.
* **Real-time**: **Server-Sent Events (SSE)** for streaming AI outputs (typing effect).
* **State/Cache**: Redis (with in-memory fallback).
* **Infrastructure**: Docker Compose.

### Directory Structure

unanimo-ai/
├── cmd/server/          # Entry point — dependency wiring
├── internal/
│   ├── domain/          # Entities (Game, Word, Score) & Scoring Logic
│   ├── ports/           # Interfaces (LLMPlayer, Judge, Repository)
│   ├── service/         # GameService — orchestration logic
│   ├── adapters/
│   │   ├── llm/         # OpenAI, Anthropic, Gemini, Groq + Judge Implementation
│   │   ├── storage/     # Redis + in-memory fallback
│   │   └── http/        # HTMX handlers, SSE broadcaster, router
└── web/
├── templates/       # Go HTML templates (HTMX snippets)
└── static/          # manifest.json, sw.js, icons (PWA)

## 4. Implementation Instructions for Developer LLM

### Phase 1: Domain & Concurrency
- Define the `LLMPlayer` port.
- Implement the `GameService` to trigger 5 Goroutines simultaneously to fetch 8 words from each API.
- Standardize all AI calls to `temperature: 0.7`.

### Phase 2: Real-time UI (HTMX + SSE)
- Build the main board with 5 columns.
- Use SSE to push `word-added` events so the user sees words appear in real-time.
- Show "Strict" matches (exact strings) immediately.
- Once the "Judge" finishes, send a `judge-complete` event via SSE to update the UI with "Semantic" clusters and final scores.

### Phase 3: Configuration & Custom Player
- Implement a `.env` loader for API keys.
- The "Custom Slot" allows a user-defined system prompt/persona (e.g., "Answer like a 19th-century poet").

## 5. Judge LLM Integration
After all 40 words are generated, the backend calls the Judge:
* **System Prompt**: "Group these 40 words into semantic clusters. Words with the same meaning or root (e.g., 'Newspaper'/'Newspapers') must share a unique `cluster_id`. Return a JSON map: `{"word": cluster_id}`."
* **Implementation**: Backend maps raw results to these IDs before final scoring.

## 6. Deployment (Docker Compose)
From the repo root: **`make start`**. The app is served at **http://localhost:8888** by default (`APP_HOST_PORT` in `.env` overrides the host side; the container still uses port 8080 internally).

```yaml
services:
  app:
    build: .
    ports: ["8888:8080"]   # host:container (default avoids busy 8080 on the host)
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - GEMINI_API_KEY=${GEMINI_API_KEY}
      - GROQ_API_KEY=${GROQ_API_KEY}
    depends_on: [redis]
  redis:
    image: redis:7-alpine
```

## 7. API Endpoints
| Method | Path | Description |
|:-------|:-----|:------------|
| **GET** | `/` | Main landing page: displays 10 random concepts. |
| **POST** | `/api/game` | Initializes a new game session and triggers LLM Goroutines. |
| **GET** | `/game/{id}` | The main game board view. |
| **GET** | `/api/game/{id}/stream` | **SSE endpoint**: Streams real-time words and score updates. |
| **GET** | `/api/game/{id}/state` | Returns JSON of current game state (scores, words). |
| **GET** | `/api/concepts` | Refresh the 10 random concepts via HTMX. |
| **POST** | `/api/persona` | Updates the System Prompt for the Custom Player slot. |
