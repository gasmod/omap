package omap_test

import (
	"fmt"
	"iter"
	"math/rand/v2"
	"slices"
	"testing"
)

// orderedMap is the method set Map and SyncMap share, so both run the same
// conformance suites.
type orderedMap[K comparable, V any] interface {
	Len() int
	Has(key K) bool
	Load(key K) (V, bool)
	Store(key K, value V)
	Clear()
	LoadOrStore(key K, value V) (V, bool)
	LoadAndDelete(key K) (V, bool)
	Delete(key K)
	Swap(key K, value V) (V, bool)
	CompareAndSwap(key K, value V, comp func(old, value V) bool) bool
	CompareAndDelete(key K, comp func(old V) bool) bool
	Range(f func(key K, value V) bool)
	All() iter.Seq2[K, V]
	Keys() iter.Seq[K]
	Values() iter.Seq[V]
}

// entry is a key-value pair as a map should produce it.
type entry struct {
	key   string
	value int
}

// runConformance checks the behavior Map and SyncMap share, using string keys.
// Builtin maps are the reference wherever their semantics overlap.
func runConformance(t *testing.T, newMap func() orderedMap[string, int]) {
	t.Helper()

	tests := []struct {
		run  func(t *testing.T, m orderedMap[string, int])
		name string
	}{
		{name: "ZeroValue", run: testZeroValue},
		{name: "InsertionOrder", run: testInsertionOrder},
		{name: "Clear", run: testClear},
		{name: "LoadOrStore", run: testLoadOrStore},
		{name: "LoadAndDelete", run: testLoadAndDelete},
		{name: "Swap", run: testSwap},
		{name: "CompareAndSwap", run: testCompareAndSwap},
		{name: "CompareAndDelete", run: testCompareAndDelete},
		{name: "RandomOpsMatchModel", run: testRandomOpsMatchModel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { tt.run(t, newMap()) })
	}
	t.Run("EarlyBreak", func(t *testing.T) { testEarlyBreak(t, newMap) })
	t.Run("MutateDuringIteration", func(t *testing.T) { testMutateDuringIteration(t, newMap) })
}

func testZeroValue(t *testing.T, m orderedMap[string, int]) {
	assertEntries(t, m, nil)
	if m.Has("a") {
		t.Fatal("Has(a) = true on an empty map")
	}
	if v, ok := m.Load("a"); ok || v != 0 {
		t.Fatalf("Load(a) = (%d, %v), want (0, false)", v, ok)
	}

	// Every mutating method works on the zero value.
	m.Delete("a")
	m.Clear()
	m.Store("a", 1)
	assertEntries(t, m, []entry{{"a", 1}})
}

func testInsertionOrder(t *testing.T, m orderedMap[string, int]) {
	m.Store("c", 1)
	m.Store("a", 2)
	m.Store("b", 3)
	assertEntries(t, m, []entry{{"c", 1}, {"a", 2}, {"b", 3}})

	// Overwriting keeps the original position.
	m.Store("c", 10)
	assertEntries(t, m, []entry{{"c", 10}, {"a", 2}, {"b", 3}})

	// Deleting and storing again moves the key to the end.
	m.Delete("c")
	m.Store("c", 20)
	assertEntries(t, m, []entry{{"a", 2}, {"b", 3}, {"c", 20}})

	if m.Has("missing") {
		t.Fatal("Has(missing) = true")
	}
}

func testClear(t *testing.T, m orderedMap[string, int]) {
	m.Store("a", 1)
	m.Store("b", 2)
	m.Clear()
	assertEntries(t, m, nil)
	if m.Has("a") {
		t.Fatal("Has(a) = true after Clear")
	}

	// The map is usable after Clear, and the old order is forgotten.
	m.Store("b", 3)
	m.Store("a", 4)
	assertEntries(t, m, []entry{{"b", 3}, {"a", 4}})
}

func testLoadOrStore(t *testing.T, m orderedMap[string, int]) {
	if v, loaded := m.LoadOrStore("a", 1); loaded || v != 1 {
		t.Fatalf("LoadOrStore(a, 1) on missing key = (%d, %v), want (1, false)", v, loaded)
	}
	if v, loaded := m.LoadOrStore("a", 2); !loaded || v != 1 {
		t.Fatalf("LoadOrStore(a, 2) on present key = (%d, %v), want (1, true)", v, loaded)
	}
	assertEntries(t, m, []entry{{"a", 1}})
}

func testLoadAndDelete(t *testing.T, m orderedMap[string, int]) {
	m.Store("a", 1)
	m.Store("b", 2)

	if v, loaded := m.LoadAndDelete("a"); !loaded || v != 1 {
		t.Fatalf("LoadAndDelete(a) on present key = (%d, %v), want (1, true)", v, loaded)
	}
	if v, loaded := m.LoadAndDelete("a"); loaded || v != 0 {
		t.Fatalf("LoadAndDelete(a) on missing key = (%d, %v), want (0, false)", v, loaded)
	}
	assertEntries(t, m, []entry{{"b", 2}})
}

func testSwap(t *testing.T, m orderedMap[string, int]) {
	m.Store("a", 1)
	m.Store("b", 2)

	if prev, loaded := m.Swap("a", 10); !loaded || prev != 1 {
		t.Fatalf("Swap(a, 10) on present key = (%d, %v), want (1, true)", prev, loaded)
	}
	// Unlike sync.Map, Swap does not store a missing key.
	if prev, loaded := m.Swap("missing", 3); loaded || prev != 0 {
		t.Fatalf("Swap(missing, 3) = (%d, %v), want (0, false)", prev, loaded)
	}
	assertEntries(t, m, []entry{{"a", 10}, {"b", 2}})
}

func testCompareAndSwap(t *testing.T, m orderedMap[string, int]) {
	m.Store("a", 1)
	m.Store("b", 2)

	var gotOld, gotNew int
	swapped := m.CompareAndSwap("a", 5, func(old, value int) bool {
		gotOld, gotNew = old, value
		return true
	})
	if !swapped || gotOld != 1 || gotNew != 5 {
		t.Fatalf("CompareAndSwap(a, 5) = %v with comp(%d, %d), want true with comp(1, 5)",
			swapped, gotOld, gotNew)
	}

	if m.CompareAndSwap("a", 6, func(int, int) bool { return false }) {
		t.Fatal("CompareAndSwap(a, 6) swapped although comp returned false")
	}
	if m.CompareAndSwap("missing", 6, func(int, int) bool { return true }) {
		t.Fatal("CompareAndSwap(missing, 6) swapped a missing key")
	}
	assertEntries(t, m, []entry{{"a", 5}, {"b", 2}})
}

func testCompareAndDelete(t *testing.T, m orderedMap[string, int]) {
	m.Store("a", 1)
	m.Store("b", 2)

	if m.CompareAndDelete("a", func(int) bool { return false }) {
		t.Fatal("CompareAndDelete(a) deleted although comp returned false")
	}
	if m.CompareAndDelete("missing", func(int) bool { return true }) {
		t.Fatal("CompareAndDelete(missing) reported deleting a missing key")
	}

	var gotOld int
	deleted := m.CompareAndDelete("a", func(old int) bool {
		gotOld = old
		return true
	})
	if !deleted || gotOld != 1 {
		t.Fatalf("CompareAndDelete(a) = %v with comp(%d), want true with comp(1)", deleted, gotOld)
	}
	assertEntries(t, m, []entry{{"b", 2}})
}

// iterationMethods lists every way to iterate a map, each adapted to report
// only the values it produces. Keys produces no values, so it reports the
// value fill stores for each key instead.
var iterationMethods = []struct {
	iterate func(m orderedMap[string, int], f func(value int) bool)
	name    string
}{
	{name: "Range", iterate: func(m orderedMap[string, int], f func(int) bool) {
		m.Range(func(_ string, v int) bool { return f(v) })
	}},
	{name: "All", iterate: func(m orderedMap[string, int], f func(int) bool) {
		for _, v := range m.All() {
			if !f(v) {
				return
			}
		}
	}},
	{name: "Keys", iterate: func(m orderedMap[string, int], f func(int) bool) {
		for k := range m.Keys() {
			if !f(fillValue(k)) {
				return
			}
		}
	}},
	{name: "Values", iterate: func(m orderedMap[string, int], f func(int) bool) {
		for v := range m.Values() {
			if !f(v) {
				return
			}
		}
	}},
}

// fill stores each single-letter key with its fillValue, so a key alone
// identifies its value.
func fill(m orderedMap[string, int], keys ...string) orderedMap[string, int] {
	for _, k := range keys {
		m.Store(k, fillValue(k))
	}
	return m
}

// fillValue is the value fill stores for a single-letter key: a=1, b=2, and
// so on.
func fillValue(k string) int {
	return int(k[0]-'a') + 1
}

func testEarlyBreak(t *testing.T, newMap func() orderedMap[string, int]) {
	for _, method := range iterationMethods {
		m := fill(newMap(), "a", "b", "c")
		calls := 0
		method.iterate(m, func(int) bool {
			calls++
			return false
		})
		if calls != 1 {
			t.Errorf("%s: f called %d times after returning false, want 1", method.name, calls)
		}
	}
}

// mutationsDuringIteration each change a map holding a=1, b=2, c=3, d=4 when
// iteration reaches a, and list the values a builtin map produces.
var mutationsDuringIteration = []struct {
	name   string
	mutate func(m orderedMap[string, int])
	want   []int
	// needsValues skips Keys, which cannot observe a changed value.
	needsValues bool
}{
	{
		// Deleting the current entry does not disturb the rest.
		name:   "DeleteCurrent",
		mutate: func(m orderedMap[string, int]) { m.Delete("a") },
		want:   []int{1, 2, 3, 4},
	},
	{
		// An entry deleted before it is reached is not produced.
		name:   "DeleteUnreached",
		mutate: func(m orderedMap[string, int]) { m.Delete("c") },
		want:   []int{1, 2, 4},
	},
	{
		// An entry is produced with its value when reached, not when
		// iteration started.
		name:        "UpdateUnreached",
		mutate:      func(m orderedMap[string, int]) { m.Store("c", 30) },
		want:        []int{1, 2, 30, 4},
		needsValues: true,
	},
	{
		// Clear removes every unreached entry.
		name:   "Clear",
		mutate: func(m orderedMap[string, int]) { m.Clear() },
		want:   []int{1},
	},
}

func testMutateDuringIteration(t *testing.T, newMap func() orderedMap[string, int]) {
	for _, tt := range mutationsDuringIteration {
		t.Run(tt.name, func(t *testing.T) {
			for _, method := range iterationMethods {
				if tt.needsValues && method.name == "Keys" {
					continue
				}
				m := fill(newMap(), "a", "b", "c", "d")
				var got []int
				method.iterate(m, func(v int) bool {
					got = append(got, v)
					if v == 1 {
						tt.mutate(m)
					}
					return true
				})
				if !slices.Equal(got, tt.want) {
					t.Errorf("%s produced %v, want %v", method.name, got, tt.want)
				}
			}
		})
	}
}

// model is a deliberately naive ordered map used as the reference for the
// random operations test.
type model struct {
	values map[string]int
	order  []string
}

func newModel() *model {
	return &model{values: map[string]int{}}
}

func (md *model) has(k string) bool {
	_, ok := md.values[k]
	return ok
}

func (md *model) store(k string, v int) {
	if !md.has(k) {
		md.order = append(md.order, k)
	}
	md.values[k] = v
}

func (md *model) delete(k string) {
	if !md.has(k) {
		return
	}
	delete(md.values, k)
	md.order = slices.DeleteFunc(md.order, func(o string) bool { return o == k })
}

func (md *model) clear() {
	md.values, md.order = map[string]int{}, nil
}

func (md *model) entries() []entry {
	es := make([]entry, 0, len(md.order))
	for _, k := range md.order {
		es = append(es, entry{k, md.values[k]})
	}
	return es
}

func isEven(v int) bool { return v%2 == 0 }

// randomOps are the operations the random test applies. Each one applies an
// operation to the map under test and decides its effect on the model from
// the model's own state, not from what the map returns.
var randomOps = []struct {
	apply func(m orderedMap[string, int], md *model, k string, v int)
	name  string
}{
	{name: "Store", apply: func(m orderedMap[string, int], md *model, k string, v int) {
		m.Store(k, v)
		md.store(k, v)
	}},
	{name: "Delete", apply: func(m orderedMap[string, int], md *model, k string, _ int) {
		m.Delete(k)
		md.delete(k)
	}},
	{name: "LoadOrStore", apply: func(m orderedMap[string, int], md *model, k string, v int) {
		m.LoadOrStore(k, v)
		if !md.has(k) {
			md.store(k, v)
		}
	}},
	{name: "LoadAndDelete", apply: func(m orderedMap[string, int], md *model, k string, _ int) {
		m.LoadAndDelete(k)
		md.delete(k)
	}},
	{name: "Swap", apply: func(m orderedMap[string, int], md *model, k string, v int) {
		m.Swap(k, v)
		if md.has(k) {
			md.store(k, v)
		}
	}},
	{name: "CompareAndSwap", apply: func(m orderedMap[string, int], md *model, k string, v int) {
		m.CompareAndSwap(k, v, func(old, _ int) bool { return isEven(old) })
		if md.has(k) && isEven(md.values[k]) {
			md.store(k, v)
		}
	}},
	{name: "CompareAndDelete", apply: func(m orderedMap[string, int], md *model, k string, _ int) {
		m.CompareAndDelete(k, isEven)
		if md.has(k) && isEven(md.values[k]) {
			md.delete(k)
		}
	}},
}

func testRandomOpsMatchModel(t *testing.T, m orderedMap[string, int]) {
	md := newModel()
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // fixed seed keeps failures reproducible

	for step := range 5000 {
		op := applyRandomOp(rng, m, md)
		if got, want := collect(m), md.entries(); !slices.Equal(got, want) {
			t.Fatalf("step %d, after %s: entries = %v, want %v", step, op, got, want)
		}
	}
}

// applyRandomOp applies one random operation to both m and md and returns a
// description of it. Clear is rare so the map has time to grow between
// clears.
func applyRandomOp(rng *rand.Rand, m orderedMap[string, int], md *model) string {
	if rng.IntN(300) == 0 {
		m.Clear()
		md.clear()
		return "Clear()"
	}

	keys := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	op := randomOps[rng.IntN(len(randomOps))]
	k, v := keys[rng.IntN(len(keys))], rng.IntN(100)
	op.apply(m, md, k, v)
	return fmt.Sprintf("%s(%q, %d)", op.name, k, v)
}

// testClone checks that clone copies m's entries in order, and that the
// clone and m are independent afterwards.
func testClone[M orderedMap[string, int]](t *testing.T, newMap func() M, clone func(M) M) {
	t.Run("Independent", func(t *testing.T) {
		m := newMap()
		m.Store("a", 1)
		m.Store("b", 2)

		c := clone(m)
		assertEntries(t, c, []entry{{"a", 1}, {"b", 2}})

		c.Store("c", 3)
		c.Store("a", 10)
		c.Delete("b")
		assertEntries(t, m, []entry{{"a", 1}, {"b", 2}})

		m.Store("d", 4)
		assertEntries(t, c, []entry{{"a", 10}, {"c", 3}})
	})

	t.Run("ZeroValue", func(t *testing.T) {
		m := newMap()
		c := clone(m)
		c.Store("a", 1)
		assertEntries(t, c, []entry{{"a", 1}})
		assertEntries(t, m, nil)
	})
}

// collect returns the entries All produces, in order.
func collect(m orderedMap[string, int]) []entry {
	var es []entry
	for k, v := range m.All() {
		es = append(es, entry{k, v})
	}
	return es
}

// assertEntries checks that Len, All, Range, Keys, Values and Load all agree
// with want.
func assertEntries(t *testing.T, m orderedMap[string, int], want []entry) {
	t.Helper()

	if m.Len() != len(want) {
		t.Fatalf("Len = %d, want %d", m.Len(), len(want))
	}
	if got := collect(m); !slices.Equal(got, want) {
		t.Fatalf("All = %v, want %v", got, want)
	}

	var ranged []entry
	m.Range(func(k string, v int) bool {
		ranged = append(ranged, entry{k, v})
		return true
	})
	if !slices.Equal(ranged, want) {
		t.Fatalf("Range = %v, want %v", ranged, want)
	}

	wantKeys := make([]string, 0, len(want))
	wantValues := make([]int, 0, len(want))
	for _, e := range want {
		wantKeys = append(wantKeys, e.key)
		wantValues = append(wantValues, e.value)
		if v, ok := m.Load(e.key); !ok || v != e.value {
			t.Fatalf("Load(%q) = (%d, %v), want (%d, true)", e.key, v, ok, e.value)
		}
	}
	if got := slices.Collect(m.Keys()); !slices.Equal(got, wantKeys) {
		t.Fatalf("Keys = %v, want %v", got, wantKeys)
	}
	if got := slices.Collect(m.Values()); !slices.Equal(got, wantValues) {
		t.Fatalf("Values = %v, want %v", got, wantValues)
	}
}
