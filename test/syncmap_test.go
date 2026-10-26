package omap_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/gasmod/omap"
)

const (
	// goroutines is how many goroutines each concurrency test races.
	goroutines = 8
	// trials is how many times each atomicity test repeats its race, since
	// a single race rarely interleaves badly.
	trials = 500
)

func TestSyncMap(t *testing.T) {
	runConformance(t, func() orderedMap[string, int] { return &omap.SyncMap[string, int]{} })
}

func TestSyncMapFloatKeys(t *testing.T) {
	runFloatKeyConformance(t, func() orderedMap[float64, int] {
		return &omap.SyncMap[float64, int]{}
	})
}

func TestSyncMapClone(t *testing.T) {
	testClone(t,
		func() *omap.SyncMap[string, int] { return &omap.SyncMap[string, int]{} },
		(*omap.SyncMap[string, int]).Clone,
	)
}

func TestSyncMapConcurrentIterationAndWrites(_ *testing.T) {
	// This test has no assertions; run it under -race, which reports any
	// access to the map's state without the lock.
	var m omap.SyncMap[int, int]
	for i := range 100 {
		m.Store(i, i)
	}

	race(goroutines, func(i int) {
		for j := range 200 {
			key := j % 100
			switch i % 4 {
			case 0: // writer
				m.Store(key, j)
			case 1: // iterators
				m.Range(func(int, int) bool { return true })
				for range m.All() {
				}
			case 2: // more iterators, and Clone
				for range m.Values() {
				}
				m.Clone()
			case 3: // deleter, with an occasional Clear
				if j%50 == 0 {
					m.Clear()
				}
				m.Delete(key)
			}
		}
	})
}

func TestSyncMapLoadOrStoreIsAtomic(t *testing.T) {
	// Exactly one goroutine stores, and every goroutine gets back the value
	// that ends up in the map.
	for trial := range trials {
		var m omap.SyncMap[string, int]
		stored := make([]bool, goroutines)
		actual := make([]int, goroutines)
		race(goroutines, func(i int) {
			v, loaded := m.LoadOrStore("k", i)
			actual[i], stored[i] = v, !loaded
		})

		if n := countTrue(stored); n != 1 {
			t.Fatalf("trial %d: %d goroutines stored, want exactly 1", trial, n)
		}
		final, _ := m.Load("k")
		for i, v := range actual {
			if v != final {
				t.Fatalf("trial %d: goroutine %d got %d, but the map holds %d", trial, i, v, final)
			}
		}
	}
}

func TestSyncMapLoadAndDeleteIsAtomic(t *testing.T) {
	// Exactly one goroutine gets the deleted value.
	for trial := range trials {
		var m omap.SyncMap[string, int]
		m.Store("k", 1)
		loaded := make([]bool, goroutines)
		race(goroutines, func(i int) {
			_, loaded[i] = m.LoadAndDelete("k")
		})

		if n := countTrue(loaded); n != 1 {
			t.Fatalf("trial %d: %d goroutines loaded the deleted value, want exactly 1", trial, n)
		}
	}
}

func TestSyncMapSwapIsAtomic(t *testing.T) {
	// Every value ever stored is handed out exactly once: either as some
	// Swap's previous value or as the final value.
	want := make([]int, goroutines+1) // -1, 0, 1, ..., goroutines-1
	for i := range want {
		want[i] = i - 1
	}

	for trial := range trials {
		var m omap.SyncMap[string, int]
		m.Store("k", -1)
		got := make([]int, goroutines, goroutines+1)
		race(goroutines, func(i int) {
			got[i], _ = m.Swap("k", i)
		})

		final, _ := m.Load("k")
		got = append(got, final)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatalf("trial %d: previous values plus final value = %v, want %v", trial, got, want)
		}
	}
}

func TestSyncMapCompareAndSwapCounter(t *testing.T) {
	// A counter incremented with a CompareAndSwap retry loop loses no
	// updates.
	const increments = 200
	var m omap.SyncMap[string, int]
	m.Store("n", 0)

	race(goroutines, func(int) {
		for range increments {
			for {
				cur, _ := m.Load("n")
				if m.CompareAndSwap("n", cur+1, func(old, _ int) bool { return old == cur }) {
					break
				}
			}
		}
	})

	if got, ok := m.Load("n"); !ok || got != goroutines*increments {
		t.Fatalf("Load(n) = (%d, %v), want (%d, true)", got, ok, goroutines*increments)
	}
}

func TestSyncMapCompareAndDeleteIsAtomic(t *testing.T) {
	// comp and the delete happen together, so a value stored between them
	// cannot be deleted by mistake.
	for trial := range trials {
		var m omap.SyncMap[string, int]
		m.Store("k", 0)
		race(2, func(i int) {
			if i == 0 {
				m.CompareAndDelete("k", func(old int) bool { return old == 0 })
			} else {
				m.Store("k", 1)
			}
		})

		// Either the delete ran first and Store added 1 back, or Store ran
		// first and comp rejected 1. Both leave k=1.
		if v, ok := m.Load("k"); !ok || v != 1 {
			t.Fatalf("trial %d: Load(k) = (%d, %v), want (1, true)", trial, v, ok)
		}
	}
}

// race starts n goroutines that run f as close to simultaneously as possible,
// passing each its index, and waits for all of them.
func race(n int, f func(i int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Go(func() {
			<-start
			f(i)
		})
	}
	close(start)
	wg.Wait()
}

func countTrue(bs []bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}
