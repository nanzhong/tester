package db

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/nanzhong/tester"
	"golang.org/x/sync/errgroup"
)

const (
	recentPrefix = "recent"
	testPrefix   = "tests"
	runPrefix    = "runs"

	defaultRecentPeriod = 3 * time.Hour
	runHistory          = 30 * time.Minute
)

type S3Option func(*s3Options)

type s3Options struct {
	recentPeriod time.Duration
}

type S3 struct {
	s3           *s3.S3
	bucket       string
	recentPeriod time.Duration
}

func NewS3(config *aws.Config, bucket string, opts ...S3Option) *S3 {
	defOpts := &s3Options{
		recentPeriod: defaultRecentPeriod,
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	return &S3{
		s3:           s3.New(session.New(config)),
		bucket:       bucket,
		recentPeriod: defOpts.recentPeriod,
	}
}

func (s *S3) AddTest(ctx context.Context, test *tester.Test) error {
	testJSON, err := json.Marshal(test)
	if err != nil {
		return fmt.Errorf("serializing test results: %w", err)
	}

	var eg errgroup.Group
	eg.Go(func() error {
		_, err = s.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
			Body:   bytes.NewReader(testJSON),
			Bucket: &s.bucket,
			Key:    keyForTest(test.ID),
		})
		return err
	})
	eg.Go(func() error {
		_, err = s.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
			Body:   bytes.NewReader(testJSON),
			Bucket: &s.bucket,
			Key:    recentKey(keyForTest(test.ID)),
		})
		return err
	})
	err = eg.Wait()
	if err != nil {
		return fmt.Errorf("adding test result: %w", err)
	}

	return nil
}

func (s *S3) GetTest(ctx context.Context, id string) (*tester.Test, error) {
	obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    keyForTest(id),
	})
	if err != nil {
		return nil, fmt.Errorf("getting test result: %w", err)
	}

	var test tester.Test
	err = json.NewDecoder(obj.Body).Decode(&test)
	if err != nil {
		return nil, fmt.Errorf("parsing test result: %w", err)
	}

	return &test, nil
}

func (s *S3) ListTests(ctx context.Context) ([]*tester.Test, error) {
	testPrefix := testPrefix
	objs, err := s.s3.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: recentKey(&testPrefix),
	})
	if err != nil {
		return nil, fmt.Errorf("listing test results: %w", err)
	}

	var (
		tests []*tester.Test
		mu    sync.Mutex
		eg    errgroup.Group
	)

	for _, obj := range objs.Contents {
		key := obj.Key
		eg.Go(func() error {
			obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
				Bucket: &s.bucket,
				Key:    key,
			})
			if err != nil {
				return err
			}

			var test tester.Test
			err = json.NewDecoder(obj.Body).Decode(&test)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()
			tests = append(tests, &test)
			return nil
		})
	}
	err = eg.Wait()
	if err != nil {
		return nil, fmt.Errorf("listing test results: %w", err)
	}

	sort.Slice(tests, func(i int, j int) bool {
		return tests[i].FinishedAt.After(tests[j].FinishedAt)
	})

	return tests, nil
}

func (s *S3) Archive(ctx context.Context) error {
	testPrefix := testPrefix
	objs, err := s.s3.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: recentKey(&testPrefix),
	})
	if err != nil {
		return fmt.Errorf("listing recent test result for archiving: %w", err)
	}

	var (
		keysToDelete []*s3.ObjectIdentifier
		mu           sync.Mutex
		eg           errgroup.Group
	)

	for _, obj := range objs.Contents {
		key := obj.Key
		eg.Go(func() error {
			obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
				Bucket: &s.bucket,
				Key:    key,
			})
			if err != nil {
				return err
			}

			var test tester.Test
			err = json.NewDecoder(obj.Body).Decode(&test)
			if err != nil {
				return err
			}

			if time.Now().Sub(test.FinishedAt) > s.recentPeriod {
				mu.Lock()
				defer mu.Unlock()
				keysToDelete = append(keysToDelete, &s3.ObjectIdentifier{
					Key: recentKey(keyForTest(test.ID)),
				})
			}
			return nil
		})
	}
	err = eg.Wait()
	if err != nil {
		return fmt.Errorf("listing test results to archive: %w", err)
	}

	if len(keysToDelete) == 0 {
		return nil
	}

	log.Printf("archiving %d test results", len(keysToDelete))
	_, err = s.s3.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
		Bucket: &s.bucket,
		Delete: &s3.Delete{
			Objects: keysToDelete,
		},
	})
	if err != nil {
		return fmt.Errorf("archiving test results: %w", err)
	}

	// TODO archive historic finished runs as well...
	return nil
}

func (s *S3) EnqueueRun(ctx context.Context, run *tester.Run) error {
	runJSON, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}

	_, err = s.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Body:   bytes.NewReader(runJSON),
		Bucket: &s.bucket,
		Key:    keyForRun(run.ID),
	})
	if err != nil {
		return fmt.Errorf("enqueueing run %w", err)
	}

	return nil
}

func (s *S3) StartRun(ctx context.Context, id string) error {
	obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    keyForRun(id),
	})
	if err != nil {
		return fmt.Errorf("getting run: %w", err)
	}

	var run tester.Run
	err = json.NewDecoder(obj.Body).Decode(&run)
	if err != nil {
		return fmt.Errorf("parsing run: %w", err)
	}

	run.StartedAt = time.Now()

	runJSON, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}
	_, err = s.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Body:   bytes.NewReader(runJSON),
		Bucket: &s.bucket,
		Key:    keyForRun(run.ID),
	})
	if err != nil {
		return fmt.Errorf("updating run started at: %w", err)
	}

	return nil
}

func (s *S3) ResetRun(ctx context.Context, id string) error {
	obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    keyForRun(id),
	})
	if err != nil {
		return fmt.Errorf("getting run: %w", err)
	}

	var run tester.Run
	err = json.NewDecoder(obj.Body).Decode(&run)
	if err != nil {
		return fmt.Errorf("parsing run: %w", err)
	}

	run.StartedAt = time.Time{}

	runJSON, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}
	_, err = s.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Body:   bytes.NewReader(runJSON),
		Bucket: &s.bucket,
		Key:    keyForRun(run.ID),
	})
	if err != nil {
		return fmt.Errorf("resetting run started at: %w", err)
	}

	return nil
}

func (s *S3) CompleteRun(ctx context.Context, id string, testIDs []string) error {
	obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    keyForRun(id),
	})
	if err != nil {
		return fmt.Errorf("getting run: %w", err)
	}

	var run tester.Run
	err = json.NewDecoder(obj.Body).Decode(&run)
	if err != nil {
		return fmt.Errorf("parsing run: %w", err)
	}

	run.FinishedAt = time.Now()

	runJSON, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("serializing run: %w", err)
	}
	_, err = s.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Body:   bytes.NewReader(runJSON),
		Bucket: &s.bucket,
		Key:    keyForRun(run.ID),
	})
	if err != nil {
		return fmt.Errorf("completing run: %w", err)
	}

	// TODO lookup tests by ids and save

	return nil
}

func (s *S3) ListRuns(ctx context.Context) ([]*tester.Run, error) {
	runPrefix := runPrefix
	objs, err := s.s3.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &runPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}

	var (
		runs []*tester.Run
		mu   sync.Mutex
		eg   errgroup.Group
	)

	for _, obj := range objs.Contents {
		key := obj.Key
		eg.Go(func() error {
			obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
				Bucket: &s.bucket,
				Key:    key,
			})
			if err != nil {
				return err
			}

			var run tester.Run
			err = json.NewDecoder(obj.Body).Decode(&run)
			if err != nil {
				return err
			}
			if !run.FinishedAt.IsZero() {
				return nil
			}

			mu.Lock()
			defer mu.Unlock()
			runs = append(runs, &run)

			return nil
		})
	}
	err = eg.Wait()
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}

	return runs, nil
}

func (s *S3) GetRun(ctx context.Context, id string) (*tester.Run, error) {
	obj, err := s.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    keyForRun(id),
	})
	if err != nil {
		return nil, fmt.Errorf("getting run: %w", err)
	}

	var run tester.Run
	err = json.NewDecoder(obj.Body).Decode(&run)
	if err != nil {
		return nil, fmt.Errorf("deserializing run: %w", err)
	}

	return &run, nil
}

func keyForTest(id string) *string {
	key := fmt.Sprintf("%s/%s", testPrefix, id)
	return &key
}

func keyForRun(id string) *string {
	key := fmt.Sprintf("%s/%s", runPrefix, id)
	return &key
}

func recentKey(key *string) *string {
	recentKey := fmt.Sprintf("%s/%s", recentPrefix, *key)
	return &recentKey
}
