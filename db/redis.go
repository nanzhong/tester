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
	redisTestRecentLimit     = 500
	redisTestRetentionPeriod = 30 * 24 * time.Hour
	redisRunFinishedLimit    = 100
	redisRunRetentionPeriod  = 7 * 24 * time.Hour

	redisPrefixTest     = "test"
	redisPrefixRun      = "run"
	redisKeyRecent      = "recent"
	redisKeyRunPending  = "pending"
	redisKeyRunFinished = "finished"
)

type Redis struct {
	client *redis.Client
}

var _ DB = (*Redis)(nil)

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
		tx.Set(redisKeyTest(test.ID), testJSON, redisTestRetentionPeriod)
		tx.LPush(redisKeyTest(redisKeyRecent), redisKeyTest(test.ID))
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

func (r *Redis) ListTests(ctx context.Context, limit int) ([]*tester.Test, error) {
	sort := &redis.Sort{
		By:  "nosort",
		Get: []string{"*"},
	}
	if limit > 0 {
		sort.Count = int64(limit)
	}
	testJSONs, err := r.client.Sort(redisKeyTest(redisKeyRecent), sort).Result()
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
	_, err := r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.LTrim(redisKeyTest(redisKeyRecent), 0, redisTestRecentLimit-1)
		tx.LTrim(redisKeyRun(redisKeyRunFinished), 0, redisRunFinishedLimit-1)
		return nil
	})
	if err != nil {
		return fmt.Errorf("archiving old tests results and runs: %w", err)
	}
	return nil
}

func (r *Redis) EnqueueRun(ctx context.Context, run *tester.Run) error {
	runJSON, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}

	_, err = r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.Set(redisKeyRun(run.ID), runJSON, redisRunRetentionPeriod)
		tx.RPush(redisKeyRun(redisKeyRunPending), redisKeyRun(run.ID))
		return nil
	})
	if err != nil {
		return fmt.Errorf("enqueueing run: %w", err)
	}
	return nil
}

func (r *Redis) StartRun(ctx context.Context, id string) error {
	runJSON, err := r.client.Get(redisKeyRun(id)).Result()
	if err != nil {
		return fmt.Errorf("getting run to start: %w", err)
	}

	var run tester.Run
	err = json.Unmarshal([]byte(runJSON), &run)
	if err != nil {
		return fmt.Errorf("deserializing run: %w", err)
	}

	run.StartedAt = time.Now()
	runJSONBytes, err := json.Marshal(&run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}

	err = r.client.Set(redisKeyRun(id), string(runJSONBytes), redisRunRetentionPeriod).Err()
	if err != nil {
		return fmt.Errorf("starting run: %w", err)
	}
	return nil
}

func (r *Redis) ResetRun(ctx context.Context, id string) error {
	runJSON, err := r.client.Get(redisKeyRun(id)).Result()
	if err != nil {
		return fmt.Errorf("getting run to reset: %w", err)
	}

	var run tester.Run
	err = json.Unmarshal([]byte(runJSON), &run)
	if err != nil {
		return fmt.Errorf("deserializing run: %w", err)
	}

	run.StartedAt = time.Time{}
	runJSONBytes, err := json.Marshal(&run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}

	err = r.client.Set(redisKeyRun(id), string(runJSONBytes), redisRunRetentionPeriod).Err()
	if err != nil {
		return fmt.Errorf("resetting run: %w", err)
	}
	return nil
}

func (r *Redis) DeleteRun(ctx context.Context, id string) error {
	_, err := r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.LRem(redisKeyRun(redisKeyRunPending), 0, redisKeyRun(id))
		tx.Del(redisKeyRun(id))
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting run: %w", err)
	}
	return nil
}

func (r *Redis) CompleteRun(ctx context.Context, id string, testIDs []string) error {
	runJSON, err := r.client.Get(redisKeyRun(id)).Result()
	if err != nil {
		return fmt.Errorf("getting run to complete: %w", err)
	}

	var run tester.Run
	err = json.Unmarshal([]byte(runJSON), &run)
	if err != nil {
		return fmt.Errorf("deserializing run: %w", err)
	}

	run.FinishedAt = time.Now()

	var testKeys []string
	for _, id := range testIDs {
		testKeys = append(testKeys, redisKeyTest(id))
	}
	testJSONs, err := r.client.MGet(testKeys...).Result()
	if err != nil {
		return fmt.Errorf("getting associated test results for run: %w", err)
	}

	for _, tj := range testJSONs {
		testJSON, ok := tj.(string)
		if !ok {
			return fmt.Errorf("got invalid test: %#v", tj)
		}
		var test tester.Test
		err := json.Unmarshal([]byte(testJSON), &test)
		if err != nil {
			return fmt.Errorf("deserializing test to associate with run: %w", err)
		}
		run.Tests = append(run.Tests, &test)
	}

	runJSONBytes, err := json.Marshal(&run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}

	_, err = r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.LRem(redisKeyRun(redisKeyRunPending), 1, redisKeyRun(id))
		tx.Set(redisKeyRun(id), string(runJSONBytes), redisRunRetentionPeriod).Err()
		tx.LPush(redisKeyRun(redisKeyRunFinished), redisKeyRun(id))
		return nil
	})
	if err != nil {
		return fmt.Errorf("completing run: %w", err)
	}
	return nil
}

func (r *Redis) FailRun(ctx context.Context, id string, errorMessage string) error {
	runJSON, err := r.client.Get(redisKeyRun(id)).Result()
	if err != nil {
		return fmt.Errorf("getting run to fail: %w", err)
	}

	var run tester.Run
	err = json.Unmarshal([]byte(runJSON), &run)
	if err != nil {
		return fmt.Errorf("deserializing run: %w", err)
	}

	run.FinishedAt = time.Now()
	run.Error = errorMessage

	runJSONBytes, err := json.Marshal(&run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}

	_, err = r.client.TxPipelined(func(tx redis.Pipeliner) error {
		tx.LRem(redisKeyRun(redisKeyRunPending), 1, redisKeyRun(id))
		tx.Set(redisKeyRun(id), string(runJSONBytes), redisRunRetentionPeriod).Err()
		tx.LPush(redisKeyRun(redisKeyRunFinished), redisKeyRun(id))
		return nil
	})
	if err != nil {
		return fmt.Errorf("failing run: %w", err)
	}
	return nil
}

func (r *Redis) ListPendingRuns(ctx context.Context) ([]*tester.Run, error) {
	runs, err := r.listRuns(ctx, redisKeyRun(redisKeyRunPending), 0)
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}
	return runs, nil
}

func (r *Redis) ListFinishedRuns(ctx context.Context, limit int) ([]*tester.Run, error) {
	runs, err := r.listRuns(ctx, redisKeyRun(redisKeyRunFinished), limit)
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}

	return runs, nil
}

func (r *Redis) listRuns(ctx context.Context, runKey string, limit int) ([]*tester.Run, error) {
	var runs []*tester.Run

	sort := &redis.Sort{
		By:  "nosort",
		Get: []string{"*"},
	}
	if limit > 0 {
		sort.Count = int64(limit)
	}
	runJSONs, err := r.client.Sort(runKey, sort).Result()
	if err != nil {
		return nil, err
	}

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

func (r *Redis) GetRun(ctx context.Context, id string) (*tester.Run, error) {
	runJSON, err := r.client.Get(redisKeyRun(id)).Result()
	if err != nil {
		return nil, fmt.Errorf("getting run: %w", err)
	}

	var run tester.Run
	err = json.Unmarshal([]byte(runJSON), &run)
	if err != nil {
		return nil, fmt.Errorf("deserializing run: %w", err)
	}
	return &run, nil
}

func redisKeyTest(id string) string {
	return fmt.Sprintf("%s:%s", redisPrefixTest, id)
}

func redisKeyRun(id string) string {
	return fmt.Sprintf("%s:%s", redisPrefixRun, id)
}
