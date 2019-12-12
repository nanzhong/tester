package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redis "github.com/go-redis/redis/v7"
	"github.com/nanzhong/tester"
)

const (
	redisRecentLimit     = 500
	redisRetentionPeriod = 30 * 24 * time.Hour

	redisPrefixTest = "test"
	redisPrefixRun  = "run"
	redisKeyRecent  = "recent"
	redisKeyAll     = "all"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{
		client: client,
	}
}

func (r *Redis) AddTest(ctx context.Context, test *tester.Test) error {
	testJSON, err := json.Marshal(test)
	if err != nil {
		return fmt.Errorf("serializing test results: %w", err)
	}

	_, err = r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.Set(redisKeyTest(test.ID), testJSON, redisRetentionPeriod)
		tx.LPush(redisKeyTest(redisKeyRecent), redisKeyTest(test.ID))
		tx.LTrim(redisKeyTest(redisKeyRecent), 0, redisRecentLimit-1)
		return nil
	})
	if err != nil {
		return fmt.Errorf("adding test: %w", err)
	}
	return nil
}

func (r *Redis) GetTest(ctx context.Context, id string) (*tester.Test, error) {
	testJSON, err := r.client.Get(redisKeyTest(id)).Result()
	if err != nil {
		return nil, fmt.Errorf("getting test result: %w", err)
	}

	var test tester.Test
	err = json.Unmarshal([]byte(testJSON), &test)
	if err != nil {
		return nil, fmt.Errorf("deserializing test result: %w", err)
	}
	return &test, nil
}

func (r *Redis) ListTests(ctx context.Context) ([]*tester.Test, error) {
	testJSONs, err := r.client.Sort(redisKeyTest(redisKeyRecent), &redis.Sort{
		By:  "nosort",
		Get: []string{"*"},
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("listing test results: %w", err)
	}

	var tests []*tester.Test
	for _, tj := range testJSONs {
		var test tester.Test
		err := json.Unmarshal([]byte(tj), &test)
		if err != nil {
			return nil, fmt.Errorf("deserializing test result: %w", err)
		}

		tests = append(tests, &test)
	}
	return tests, nil
}

func (r *Redis) Archive(ctx context.Context) error {
	// No need to explicitly archive for redis since we use expiries.
	return nil
}

func (r *Redis) EnqueueRun(ctx context.Context, run *tester.Run) error {
	runJSON, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}

	_, err = r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.Set(redisKeyRun(run.ID), runJSON, 0)
		tx.RPush(redisKeyRun(redisKeyAll), redisKeyRun(run.ID))
		return nil
	})
	if err != nil {
		return fmt.Errorf("enqueueing run: %w", err)
	}
	return nil
}

func (r *Redis) StartRun(ctx context.Context, id string) error {
	_, err := r.client.TxPipelined(func(tx redis.Pipeliner) error {
		runJSON, err := tx.Get(redisKeyRun(id)).Result()
		if err != nil {
			return err
		}

		var run tester.Run
		err = json.Unmarshal([]byte(runJSON), &run)
		if err != nil {
			return fmt.Errorf("deserializing run: %w", err)
		}

		run.StartedAt = time.Now()
		runJSONBytes, err := json.Marshal(&run)
		tx.Set(redisKeyRun(id), string(runJSONBytes), 0)
		return nil
	})
	if err != nil {
		return fmt.Errorf("starting run: %w", err)
	}
	return nil
}

func (r *Redis) ResetRun(ctx context.Context, id string) error {
	_, err := r.client.TxPipelined(func(tx redis.Pipeliner) error {
		runJSON, err := tx.Get(redisKeyRun(id)).Result()
		if err != nil {
			return err
		}

		var run tester.Run
		err = json.Unmarshal([]byte(runJSON), &run)
		if err != nil {
			return fmt.Errorf("deserializing run: %w", err)
		}

		run.StartedAt = time.Time{}
		runJSONBytes, err := json.Marshal(&run)
		tx.Set(redisKeyRun(id), string(runJSONBytes), 0)
		return nil
	})
	if err != nil {
		return fmt.Errorf("resetting run: %w", err)
	}
	return nil
}

func (r *Redis) DeleteRun(ctx context.Context, id string) error {
	_, err := r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.LRem(redisKeyRun(redisKeyAll), 1, redisKeyRun(id))
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting run: %w", err)
	}
	return nil
}

func (r *Redis) ListRuns(ctx context.Context) ([]*tester.Run, error) {
	runJSONs, err := r.client.Sort(redisKeyRun(redisKeyAll), &redis.Sort{
		By:  "nosort",
		Get: []string{"*"},
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}

	var runs []*tester.Run
	for _, rj := range runJSONs {
		var run tester.Run
		err := json.Unmarshal([]byte(rj), &run)
		if err != nil {
			return nil, fmt.Errorf("deserializing run: %w", err)
		}

		runs = append(runs, &run)
	}
	return runs, nil
}

func redisKeyTest(id string) string {
	return fmt.Sprintf("%s:%s", redisPrefixTest, id)
}

func redisKeyRun(id string) string {
	return fmt.Sprintf("%s:%s", redisPrefixRun, id)
}
