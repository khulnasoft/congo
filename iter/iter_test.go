package iter_test

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/khulnasoft/congo/iter"

	"github.com/stretchr/testify/require"
)

func ExampleIterator() {
	input := []int{1, 2, 3, 4}
	iterator := iter.Iterator[int]{
		MaxGoroutines: len(input) / 2,
	}

	iterator.ForEach(input, func(v *int) {
		if *v%2 != 0 {
			*v = -1
		}
	})

	fmt.Println(input)
	// Output:
	// [-1 2 -1 4]
}

func TestIterator(t *testing.T) {
	t.Parallel()

	t.Run("safe for reuse", func(t *testing.T) {
		t.Parallel()

		iterator := iter.Iterator[int]{MaxGoroutines: 999}

		// iter.Congourrency > numInput case that updates iter.Congourrency
		iterator.ForEachIdx([]int{1, 2, 3}, func(i int, t *int) {})

		require.Equal(t, iterator.MaxGoroutines, 999)
	})

	t.Run("allows more than defaultMaxGoroutines() congourrent tasks", func(t *testing.T) {
		t.Parallel()

		wantCongourrency := 2 * iter.DefaultMaxGoroutines()

		maxCongourrencyHit := make(chan struct{})

		tasks := make([]int, wantCongourrency)
		iterator := iter.Iterator[int]{MaxGoroutines: wantCongourrency}

		var congourrentTasks atomic.Int64
		iterator.ForEach(tasks, func(t *int) {
			n := congourrentTasks.Add(1)
			defer congourrentTasks.Add(-1)

			if int(n) == wantCongourrency {
				// All our tasks are running congourrently.
				// Signal to the rest of the tasks to stop.
				close(maxCongourrencyHit)
			} else {
				// Wait until we hit max congourrency before exiting.
				// This ensures that all tasks have been started
				// in parallel, despite being a larger input set than
				// defaultMaxGoroutines().
				<-maxCongourrencyHit
			}
		})
	})
}

func TestForEachIdx(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		f := func() {
			ints := []int{}
			iter.ForEachIdx(ints, func(i int, val *int) {
				panic("this should never be called")
			})
		}
		require.NotPanics(t, f)
	})

	t.Run("panic is propagated", func(t *testing.T) {
		t.Parallel()
		f := func() {
			ints := []int{1}
			iter.ForEachIdx(ints, func(i int, val *int) {
				panic("super bad thing happened")
			})
		}
		require.Panics(t, f)
	})

	t.Run("mutating inputs is fine", func(t *testing.T) {
		t.Parallel()
		ints := []int{1, 2, 3, 4, 5}
		iter.ForEachIdx(ints, func(i int, val *int) {
			*val += 1
		})
		require.Equal(t, []int{2, 3, 4, 5, 6}, ints)
	})

	t.Run("huge inputs", func(t *testing.T) {
		t.Parallel()
		ints := make([]int, 10000)
		iter.ForEachIdx(ints, func(i int, val *int) {
			*val = i
		})
		expected := make([]int, 10000)
		for i := 0; i < 10000; i++ {
			expected[i] = i
		}
		require.Equal(t, expected, ints)
	})
}

func TestForEach(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		f := func() {
			ints := []int{}
			iter.ForEach(ints, func(val *int) {
				panic("this should never be called")
			})
		}
		require.NotPanics(t, f)
	})

	t.Run("panic is propagated", func(t *testing.T) {
		t.Parallel()
		f := func() {
			ints := []int{1}
			iter.ForEach(ints, func(val *int) {
				panic("super bad thing happened")
			})
		}
		require.Panics(t, f)
	})

	t.Run("mutating inputs is fine", func(t *testing.T) {
		t.Parallel()
		ints := []int{1, 2, 3, 4, 5}
		iter.ForEach(ints, func(val *int) {
			*val += 1
		})
		require.Equal(t, []int{2, 3, 4, 5, 6}, ints)
	})

	t.Run("huge inputs", func(t *testing.T) {
		t.Parallel()
		ints := make([]int, 10000)
		iter.ForEach(ints, func(val *int) {
			*val = 1
		})
		expected := make([]int, 10000)
		for i := 0; i < 10000; i++ {
			expected[i] = 1
		}
		require.Equal(t, expected, ints)
	})
}

func BenchmarkForEach(b *testing.B) {
	for _, count := range []int{0, 1, 8, 100, 1000, 10000, 100000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			ints := make([]int, count)
			for i := 0; i < b.N; i++ {
				iter.ForEach(ints, func(i *int) {
					*i = 0
				})
			}
		})
	}
}
