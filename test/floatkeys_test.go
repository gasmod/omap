package omap_test

import (
	"math"
	"slices"
	"testing"
)

// runFloatKeyConformance checks the key-equality cases only float keys can
// reach: NaN, which is not equal to itself, and +0 and -0, which are equal
// but distinguishable. Builtin maps are the reference.
func runFloatKeyConformance(t *testing.T, newMap func() orderedMap[float64, int]) {
	t.Helper()

	t.Run("NaNEntries", func(t *testing.T) { testNaNEntries(t, newMap) })
	t.Run("ClearDuringIterationWithNaN", func(t *testing.T) {
		testClearDuringIterationWithNaN(t, newMap())
	})
	t.Run("ClearThenStoreNaNDuringIteration", func(t *testing.T) {
		testClearThenStoreNaNDuringIteration(t, newMap())
	})
	t.Run("SignedZeroStoreReplacesKey", func(t *testing.T) {
		testSignedZeroStoreReplacesKey(t, newMap())
	})
}

func testNaNEntries(t *testing.T, newMap func() orderedMap[float64, int]) {
	nan := math.NaN()
	tests := []struct {
		name  string
		setup func(m orderedMap[float64, int])
		want  []int
	}{
		{
			// NaN entries are produced with their stored values even though
			// they can never be looked up.
			name:  "OnlyNaN",
			setup: func(m orderedMap[float64, int]) { m.Store(nan, 7) },
			want:  []int{7},
		},
		{
			// The NaN entry produces its own value, not the first entry's.
			name: "NaNAfterOtherKey",
			setup: func(m orderedMap[float64, int]) {
				m.Store(1, 1)
				m.Store(nan, 7)
			},
			want: []int{1, 7},
		},
		{
			// NaN != NaN, so every NaN store adds an entry.
			name: "EachStoreAddsEntry",
			setup: func(m orderedMap[float64, int]) {
				m.Store(nan, 1)
				m.Store(nan, 2)
			},
			want: []int{1, 2},
		},
		{
			// Deleting an earlier key shifts the NaN entry down a slot
			// without duplicating it.
			name: "NaNAfterDeletedKey",
			setup: func(m orderedMap[float64, int]) {
				m.Store(1, 1)
				m.Store(nan, 7)
				m.Delete(1)
			},
			want: []int{7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMap()
			tt.setup(m)
			if m.Len() != len(tt.want) {
				t.Fatalf("Len = %d, want %d", m.Len(), len(tt.want))
			}
			if got := rangeValues(m, nil); !slices.Equal(got, tt.want) {
				t.Fatalf("Range produced %v, want %v", got, tt.want)
			}
		})
	}
}

func testClearDuringIterationWithNaN(t *testing.T, m orderedMap[float64, int]) {
	// Clear removes the unreached NaN entry, which must then be skipped
	// rather than looked up in the emptied map.
	m.Store(1, 1)
	m.Store(math.NaN(), 7)

	got := rangeValues(m, func(int) { m.Clear() })
	if !slices.Equal(got, []int{1}) {
		t.Fatalf("Range produced %v, want [1]", got)
	}
}

func testClearThenStoreNaNDuringIteration(t *testing.T, m orderedMap[float64, int]) {
	// A NaN stored after Clear is a new entry; it must not bring back the
	// cleared one.
	m.Store(1, 1)
	m.Store(math.NaN(), 7)

	got := rangeValues(m, func(v int) {
		if v == 1 {
			m.Clear()
			m.Store(math.NaN(), 9)
		}
	})
	if slices.Contains(got, 7) {
		t.Fatalf("Range produced %v, which includes the cleared NaN entry 7", got)
	}
}

func testSignedZeroStoreReplacesKey(t *testing.T, m orderedMap[float64, int]) {
	// +0 == -0, so the second Store overwrites the first entry. A builtin map
	// also replaces the stored key, so iteration produces -0.
	negZero := math.Copysign(0, -1)
	m.Store(0, 1)
	m.Store(negZero, 2)

	keys := slices.Collect(m.Keys())
	if len(keys) != 1 || !math.Signbit(keys[0]) {
		t.Fatalf("Keys = %v, want [-0]", keys)
	}
	if v, ok := m.Load(0); !ok || v != 2 {
		t.Fatalf("Load(0) = (%d, %v), want (2, true)", v, ok)
	}
}

// rangeValues returns the values Range produces, in order. If during is not
// nil, it is called with each value after the value is recorded.
func rangeValues(m orderedMap[float64, int], during func(v int)) []int {
	var got []int
	m.Range(func(_ float64, v int) bool {
		got = append(got, v)
		if during != nil {
			during(v)
		}
		return true
	})
	return got
}
