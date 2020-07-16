// +build example

package example

import (
	"fmt"
	"testing"
)

func TestExample_pass(t *testing.T) {
	t.Log("pass")
}

func TestExample_skip(t *testing.T) {
	t.Skip("skip")
}

func TestExample_failError(t *testing.T) {
	t.Error("error")
	t.Log("should print this")
}

func TestExample_failFatal(t *testing.T) {
	t.Fatal("fatal")
	t.Log("should not print this")
}

func TestExample_nesting(t *testing.T) {
	t.Log("running top level test")

	t.Run("subtest 1", func(t *testing.T) {
		t.Log("running subtest 1")
	})
	t.Run("subtest 2", func(t *testing.T) {
		t.Log("running subtest 2")

		t.Run("nested subtest 1", func(t *testing.T) {
			t.Log("running nested subtest 2_1")
		})

		t.Run("nested subtest 2", func(t *testing.T) {
			t.Log("running nested subtest 2_2")
		})
	})
}

func TestExample_parallel(t *testing.T) {
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("para %d", i), func(t *testing.T) {
			t.Parallel()
			t.Logf("parallel test: %d", i)
		})
	}
}

func TestExample_allStates(t *testing.T) {
	t.Log("running top level test")

	t.Run("skip subtest 1", func(t *testing.T) {
		t.Skip("skipping subtest 1")
	})
	t.Run("subtest 2", func(t *testing.T) {
		t.Log("running subtest 2")

		t.Run("fail nested subtest 1", func(t *testing.T) {
			t.Error("failing nested subtest 2_1")
		})

		t.Run("skip nested subtest 2", func(t *testing.T) {
			t.Skip("skipping nested subtest 2_2")
		})

		t.Run("nested subtest 3", func(t *testing.T) {
			t.Skip("skipping nested subtest 2_3")
		})
	})
}

func BenchmarkExample_simple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.Log("benchmarking...")
	}
}

func BenchmarkExample_nesting(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.Log("parent benchmark")
	}

	b.Run("subbench 1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.Log("running subbench 1")
		}
	})

	b.Run("subbench 2", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.Log("running subbench 2")
		}

		b.Run("nested subbench 1", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.Log("running nested subbench 2_1")
			}
		})

		b.Run("nested subbench 2", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.Log("running nested subbench 2_2")
			}
		})
	})
}

func BenchmarkExample_parallel(b *testing.B) {
	for i := 0; i < 5; i++ {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				b.Logf("parallel test: %d", i)
			}
		})
	}
}

func Simple() {}

func ExampleSimple() {
	fmt.Println("simple")
	// Output: simple
}
