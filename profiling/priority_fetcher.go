package profiling

import "github.com/go-redis/redis/v8"

type PriorityFetcher interface {
	// Fetch retrieves a priority for a session from a key-value store.
	Fetch(sessionID string) Priority
}

type RedisPriorityFetcher struct {
	client *redis.Client
}

func NewRedisPriorityFetcher(addr string, password string, db int) *RedisPriorityFetcher {
	return &RedisPriorityFetcher{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

func (f *RedisPriorityFetcher) Fetch(sessionID string) Priority {
	panic("implement me")
}
