package profiling

import (
	"fmt"
	"github.com/adjust/rmq/v3"
	"github.com/go-redis/redis/v7"
	"log"
)

type PriorityFetcher interface {
	// Profile instructs the profiler to generate a priority for a session.
	Profile(sessionID string)
	// Fetch retrieves a priority for a session from a key-value store.
	Fetch(sessionID string) (Priority, error)
}

type RedisPriorityFetcher struct {
	prioritiesClient *redis.Client
	queue            rmq.Queue
}

const RedisQueueTag = "profiler service"
const RedisQueueName = "sessions"

func NewRedisPriorityFetcher(addr string, password string, prioritiesDB int, queueDB int) (*RedisPriorityFetcher, error) {
	queueConn, err := rmq.OpenConnectionWithRedisClient(RedisQueueTag, redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       queueDB,
	}), nil)
	if err != nil {
		return nil, err
	}

	queue, err := queueConn.OpenQueue(RedisQueueName)
	if err != nil {
		return nil, err
	}

	return &RedisPriorityFetcher{
		prioritiesClient: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       prioritiesDB,
		}),
		queue: queue,
	}, nil
}

func (f *RedisPriorityFetcher) Profile(sessionID string) {
	if err := f.queue.Publish(sessionID); err != nil {
		log.Printf("could not publish session ID: %s", err)
	}
}

func (f *RedisPriorityFetcher) Fetch(sessionID string) (Priority, error) {
	val, err := f.prioritiesClient.Get(sessionID).Result()
	if err == redis.Nil {
		return Unknown, nil
	} else if err != nil {
		return Unknown, fmt.Errorf("expected rdb.Get(%s) returns nil err; got err = %w", sessionID, err)
	}

	priority, err := strToPriority(val)
	if err != nil {
		return Unknown, fmt.Errorf("expected strconv.Atoi(%s) returns nil err; got err = %w", val, err)
	}

	return priority, nil
}
