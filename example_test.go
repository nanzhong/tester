package tester

import (
	"fmt"
	"testing"
)

func TestExample_simple(t *testing.T) {
	t.Log("test for something...")
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

func ExampleSimple() {
	fmt.Println("simple")
	// Output: simple
}
