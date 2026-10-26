package omap_test

import (
	"fmt"
	"testing"

	"github.com/gasmod/omap"
)

// benchMap is the subset of methods the benchmarks exercise. Map, SyncMap and
// the builtin map reference all run through it, so each pays the same
// interface-call overhead and their results compare directly.
type benchMap interface {
	Load(key int) (int, bool)
	Store(key, value int)
	Delete(key int)
	Range(f func(key, value int) bool)
}

// builtinMap adapts a builtin map to benchMap as an unordered reference.
type builtinMap map[int]int

func (m builtinMap) Load(key int) (int, bool) {
	v, ok := m[key]
	return v, ok
}

func (m builtinMap) Store(key, value int) { m[key] = value }

func (m builtinMap) Delete(key int) { delete(m, key) }

func (m builtinMap) Range(f func(key, value int) bool) {
	for k, v := range m {
		if !f(k, v) {
			return
		}
	}
}

var benchImpls = []struct {
	newMap func() benchMap
	name   string
}{
	{name: "Map", newMap: func() benchMap { return &omap.Map[int, int]{} }},
	{name: "SyncMap", newMap: func() benchMap { return &omap.SyncMap[int, int]{} }},
	{name: "builtin", newMap: func() benchMap { return builtinMap{} }},
}

var benchSizes = []int{100, 10_000, 1_000_000}

// filled returns a map holding the keys 0 through n-1.
func filled(newMap func() benchMap, n int) benchMap {
	m := newMap()
	for i := range n {
		m.Store(i, i)
	}
	return m
}

// runSized runs bench for every implementation at every size in sizes.
func runSized(b *testing.B, sizes []int, bench func(b *testing.B, m benchMap, n int)) {
	for _, impl := range benchImpls {
		for _, n := range sizes {
			b.Run(fmt.Sprintf("%s/n=%d", impl.name, n), func(b *testing.B) {
				bench(b, filled(impl.newMap, n), n)
			})
		}
	}
}

func BenchmarkStoreNew(b *testing.B) {
	for _, impl := range benchImpls {
		b.Run(impl.name, func(b *testing.B) {
			m := impl.newMap()
			i := 0
			for b.Loop() {
				m.Store(i, i)
				i++
			}
		})
	}
}

func BenchmarkStoreExisting(b *testing.B) {
	runSized(b, benchSizes, func(b *testing.B, m benchMap, n int) {
		i := 0
		for b.Loop() {
			m.Store(i%n, i)
			i++
		}
	})
}

func BenchmarkLoadHit(b *testing.B) {
	runSized(b, benchSizes, func(b *testing.B, m benchMap, n int) {
		i := 0
		for b.Loop() {
			m.Load(i % n)
			i++
		}
	})
}

func BenchmarkLoadMiss(b *testing.B) {
	runSized(b, benchSizes, func(b *testing.B, m benchMap, n int) {
		i := 0
		for b.Loop() {
			m.Load(n + i)
			i++
		}
	})
}

// BenchmarkDeleteStore deletes a key and stores it again, so the map keeps
// its size across iterations. The stored key moves to the end of the order.
func BenchmarkDeleteStore(b *testing.B) {
	runSized(b, benchSizes, func(b *testing.B, m benchMap, n int) {
		i := 0
		for b.Loop() {
			m.Delete(i % n)
			m.Store(i%n, i)
			i++
		}
	})
}

func BenchmarkRange(b *testing.B) {
	runSized(b, benchSizes, func(b *testing.B, m benchMap, _ int) {
		for b.Loop() {
			m.Range(func(_, _ int) bool { return true })
		}
	})
}

func BenchmarkClone(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("Map/n=%d", n), func(b *testing.B) {
			m := filled(func() benchMap { return &omap.Map[int, int]{} }, n).(*omap.Map[int, int])
			for b.Loop() {
				m.Clone()
			}
		})
	}
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("SyncMap/n=%d", n), func(b *testing.B) {
			m := filled(func() benchMap { return &omap.SyncMap[int, int]{} }, n).(*omap.SyncMap[int, int])
			for b.Loop() {
				m.Clone()
			}
		})
	}
}

// BenchmarkSyncMapParallel measures SyncMap under concurrent use, with
// writePct percent of operations storing an existing key and the rest
// loading one.
func BenchmarkSyncMapParallel(b *testing.B) {
	const n = 10_000
	for _, writePct := range []int{0, 10, 50} {
		b.Run(fmt.Sprintf("writes=%d%%", writePct), func(b *testing.B) {
			m := filled(func() benchMap { return &omap.SyncMap[int, int]{} }, n)
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					if i%100 < writePct {
						m.Store(i%n, i)
					} else {
						m.Load(i % n)
					}
					i++
				}
			})
		})
	}
}
