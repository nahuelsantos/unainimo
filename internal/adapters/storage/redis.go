package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/unanimo-ai/unanimo/internal/domain"
)

const gameTTL = 2 * time.Hour

// RedisGameRepository implements ports.GameRepository using Redis
type RedisGameRepository struct {
	client *redis.Client
}

// NewRedisGameRepository creates a new Redis-backed game repository
func NewRedisGameRepository(addr, password string, db int) (*RedisGameRepository, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisGameRepository{client: client}, nil
}

func (r *RedisGameRepository) key(gameID string) string {
	return "game:" + gameID
}

// Save persists the game to Redis with a TTL
func (r *RedisGameRepository) Save(ctx context.Context, game *domain.Game) error {
	data, err := json.Marshal(game)
	if err != nil {
		return fmt.Errorf("marshal game: %w", err)
	}
	return r.client.Set(ctx, r.key(game.ID), data, gameTTL).Err()
}

// Get retrieves a game from Redis
func (r *RedisGameRepository) Get(ctx context.Context, gameID string) (*domain.Game, error) {
	data, err := r.client.Get(ctx, r.key(gameID)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("game not found: %s", gameID)
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var game domain.Game
	if err := json.Unmarshal(data, &game); err != nil {
		return nil, fmt.Errorf("unmarshal game: %w", err)
	}
	return &game, nil
}

// Delete removes a game from Redis
func (r *RedisGameRepository) Delete(ctx context.Context, gameID string) error {
	return r.client.Del(ctx, r.key(gameID)).Err()
}

// InMemoryGameRepository is a fallback repository when Redis is unavailable
type InMemoryGameRepository struct {
	games map[string]*domain.Game
}

func NewInMemoryGameRepository() *InMemoryGameRepository {
	return &InMemoryGameRepository{games: make(map[string]*domain.Game)}
}

func (r *InMemoryGameRepository) Save(_ context.Context, game *domain.Game) error {
	// Deep copy to avoid pointer aliasing
	data, _ := json.Marshal(game)
	var copy domain.Game
	json.Unmarshal(data, &copy)
	r.games[game.ID] = &copy
	return nil
}

func (r *InMemoryGameRepository) Get(_ context.Context, gameID string) (*domain.Game, error) {
	game, ok := r.games[gameID]
	if !ok {
		return nil, fmt.Errorf("game not found: %s", gameID)
	}
	// Return a copy
	data, _ := json.Marshal(game)
	var copy domain.Game
	json.Unmarshal(data, &copy)
	return &copy, nil
}

func (r *InMemoryGameRepository) Delete(_ context.Context, gameID string) error {
	delete(r.games, gameID)
	return nil
}
