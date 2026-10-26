package omap

import (
	"iter"
	"maps"
	"slices"
)

// Map is an ordered map from keys of type Key to values of type Value. It
// produces its entries in the order their keys were first stored; see the
// package documentation for the ordering and key-equality rules.
//
// Map is not safe for concurrent use. Calls that only read the map (Len,
// Has, Load, Range, All, Keys, Values and Clone) may run concurrently with
// each other, but no call may run concurrently with one that modifies the
// map. Use [SyncMap] when goroutines share a map.
//
// The zero Map is empty and ready for use. A Map must not be copied after
// first use; a copy shares part of its state with the original, and
// modifying either corrupts the other. Use [Map.Clone] to copy a Map.
type Map[Key comparable, Value any] struct {
	noCopy noCopy

	idx     map[Key]int
	entries []entry[Key, Value]
	clears  int
}

// Len returns the number of entries in m, including entries with NaN keys.
func (m *Map[Key, Value]) Len() int {
	return len(m.idx)
}

// Has reports whether key is present in m. It always reports false for a NaN
// key.
func (m *Map[Key, Value]) Has(key Key) bool {
	_, ok := m.idx[key]
	return ok
}

// Load returns the value stored in m for key, or the zero value if no value
// is present. The ok result reports whether a value was found.
func (m *Map[Key, Value]) Load(key Key) (value Value, ok bool) {
	if i, ok := m.idx[key]; ok {
		return m.entries[i].value, true
	}
	var zero Value
	return zero, false
}

// Store sets the value for key. If key is not present, a new entry is added
// at the end of m. If key is present, its entry keeps its position and both
// its value and its key are replaced, so a later iteration produces the key
// passed to the most recent Store (for example -0.0 after +0.0).
func (m *Map[Key, Value]) Store(key Key, value Value) {
	if m.idx == nil {
		m.idx = make(map[Key]int)
	}
	if i, ok := m.idx[key]; ok {
		m.entries[i].key = key // for -0 == +0
		m.entries[i].value = value
	} else {
		m.entries = append(m.entries, entry[Key, Value]{key: key, value: value})
		m.idx[key] = len(m.entries) - 1
	}
}

// Clear deletes all the entries, including entries with NaN keys, resulting
// in an empty Map. The storage used for the entries is kept for reuse.
func (m *Map[Key, Value]) Clear() {
	m.idx = make(map[Key]int)
	m.entries = slices.Delete(m.entries, 0, len(m.entries))
	m.clears++
}

// LoadOrStore returns the existing value for key if present. Otherwise, it
// stores value as Store does and returns it. The loaded result is true if
// the value was loaded, false if stored.
func (m *Map[Key, Value]) LoadOrStore(key Key, value Value) (actual Value, loaded bool) {
	if v, ok := m.Load(key); ok {
		return v, true
	}
	m.Store(key, value)
	return value, false
}

// LoadAndDelete deletes the entry for key, returning its value if any. The
// loaded result reports whether key was present.
func (m *Map[Key, Value]) LoadAndDelete(key Key) (value Value, loaded bool) {
	if v, ok := m.Load(key); ok {
		m.Delete(key)
		return v, true
	}
	var zero Value
	return zero, false
}

// Delete deletes the entry for key. It is a no-op if key is not present. The
// remaining entries keep their relative order. Delete takes time
// proportional to the number of entries in m.
func (m *Map[Key, Value]) Delete(key Key) {
	idx, ok := m.idx[key]
	if !ok {
		return
	}
	m.entries = slices.Delete(m.entries, idx, idx+1)
	m.idx = make(map[Key]int)
	for i, e := range m.entries {
		m.idx[e.key] = i
	}
}

// Swap replaces the value for key if key is present and returns the previous
// value with loaded set to true. The entry keeps its position and its key is
// replaced as in Store. If key is not present, m is unchanged and Swap
// returns the zero value and false; unlike [sync.Map.Swap], it does not store
// a missing key.
func (m *Map[Key, Value]) Swap(key Key, value Value) (previous Value, loaded bool) {
	previous, loaded = m.Load(key)
	if !loaded {
		var zero Value
		return zero, false
	}
	m.Store(key, value)
	return previous, true
}

// CompareAndSwap stores value for key if key is present and comp(old, value)
// returns true, where old is the value currently stored for key. It reports
// whether the value was swapped. comp is not called if key is not present,
// and is called at most once otherwise. comp must not modify m.
func (m *Map[Key, Value]) CompareAndSwap(
	key Key,
	value Value,
	comp func(old, value Value) bool,
) (swapped bool) {
	if v, ok := m.Load(key); ok && comp(v, value) {
		m.Store(key, value)
		return true
	}
	return false
}

// CompareAndDelete deletes the entry for key if key is present and comp(old)
// returns true, where old is the value currently stored for key. It reports
// whether the entry was deleted. comp is not called if key is not present,
// and is called at most once otherwise. comp must not modify m.
func (m *Map[Key, Value]) CompareAndDelete(
	key Key,
	comp func(old Value) bool,
) (deleted bool) {
	if v, ok := m.Load(key); ok && comp(v) {
		m.Delete(key)
		return true
	}
	return false
}

// Range calls f sequentially for each key and value in m, in insertion
// order. If f returns false, Range stops the iteration.
//
// The iteration follows the rules of ranging over a builtin map. An entry
// that is deleted before it is reached is not produced. An entry that is
// added during the iteration, including a key that is deleted and stored
// again, may be produced or skipped. Each value produced is the value stored
// for its key at the moment it is produced, and no key is produced twice.
// Entries with NaN keys are produced, unless m is cleared before they are
// reached.
//
// f may call any method of m, including ones that modify it.
//
// Range copies the list of entries before the first call to f, so it takes
// time and memory proportional to the number of entries even if f returns
// false after a constant number of calls.
func (m *Map[Key, Value]) Range(f func(key Key, value Value) bool) {
	entries := slices.Clone(m.entries)
	clears := m.clears
	for _, e := range entries {
		if _, ok := m.idx[e.key]; !ok {
			// nolint:gocritic // a NaN key wouldn't satisfy e.key == e.key
			if e.key == e.key {
				continue
			}
			if clears != m.clears {
				return
			}
		}

		cur := e
		// nolint:gocritic // a NaN key wouldn't satisfy e.key == e.key
		if e.key == e.key {
			cur = m.entries[m.idx[e.key]]
		}
		if !f(cur.key, cur.value) {
			return
		}
	}
}

// All returns an iterator over the key-value pairs in m, in insertion order.
// Each use of the iterator behaves as a call to [Map.Range].
func (m *Map[Key, Value]) All() iter.Seq2[Key, Value] {
	return func(yield func(Key, Value) bool) {
		m.Range(yield)
	}
}

// Keys returns an iterator over the keys in m, in insertion order. Each use
// of the iterator behaves as a call to [Map.Range].
func (m *Map[Key, Value]) Keys() iter.Seq[Key] {
	return func(yield func(Key) bool) {
		m.Range(func(key Key, _ Value) bool {
			return yield(key)
		})
	}
}

// Values returns an iterator over the values in m, in the insertion order of
// their keys. Each use of the iterator behaves as a call to [Map.Range].
func (m *Map[Key, Value]) Values() iter.Seq[Value] {
	return func(yield func(Value) bool) {
		m.Range(func(_ Key, value Value) bool {
			return yield(value)
		})
	}
}

// Clone returns a new Map with the same entries in the same order. It is a
// shallow clone: keys and values are copied using ordinary assignment. The
// clone and m are independent; changes to one do not affect the other.
func (m *Map[Key, Value]) Clone() *Map[Key, Value] {
	return &Map[Key, Value]{
		idx:     maps.Clone(m.idx),
		entries: slices.Clone(m.entries),
	}
}

// noCopy is embedded in Map so that go vet's copylocks check reports copies
// of a Map: a copy shares the index with the original but not the entries'
// length, so modifying either corrupts the other.
//
// Lock and Unlock are no-ops; they exist only for the copylocks check.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
