package omap

import (
	"iter"
	"maps"
	"slices"
	"sync"
)

// SyncMap is an ordered map from keys of type Key to values of type Value
// that is safe for concurrent use by multiple goroutines. It produces its
// entries in the order their keys were first stored; see the package
// documentation for the ordering and key-equality rules.
//
// All methods are guarded by a single [sync.RWMutex]: methods that only read
// the map may run in parallel, and methods that modify it run one at a time.
// Each method is atomic, including LoadOrStore, LoadAndDelete, Swap,
// CompareAndSwap and CompareAndDelete. Iteration is the exception: see
// [SyncMap.Range].
//
// The zero SyncMap is empty and ready for use. A SyncMap must not be copied
// after first use. Use [SyncMap.Clone] to copy a SyncMap.
type SyncMap[Key comparable, Value any] struct {
	idx     map[Key]int
	entries []entry[Key, Value]
	clears  int
	mu      sync.RWMutex
}

// Len returns the number of entries in m, including entries with NaN keys.
func (m *SyncMap[Key, Value]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.idx)
}

// Has reports whether key is present in m. It always reports false for a NaN
// key.
func (m *SyncMap[Key, Value]) Has(key Key) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.has(key)
}

func (m *SyncMap[Key, Value]) has(key Key) bool {
	_, ok := m.idx[key]
	return ok
}

// Load returns the value stored in m for key, or the zero value if no value
// is present. The ok result reports whether a value was found.
func (m *SyncMap[Key, Value]) Load(key Key) (value Value, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.load(key)
}

func (m *SyncMap[Key, Value]) load(key Key) (Value, bool) {
	i, ok := m.idx[key]
	if ok {
		return m.entries[i].value, true
	}
	var zero Value
	return zero, false
}

// Store sets the value for key. If key is not present, a new entry is added
// at the end of m. If key is present, its entry keeps its position and both
// its value and its key are replaced, so a later iteration produces the key
// passed to the most recent Store (for example -0.0 after +0.0).
func (m *SyncMap[Key, Value]) Store(key Key, value Value) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.store(key, value)
}

func (m *SyncMap[Key, Value]) store(key Key, value Value) {
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
// in an empty SyncMap. The storage used for the entries is kept for reuse.
func (m *SyncMap[Key, Value]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.idx = make(map[Key]int)
	m.entries = slices.Delete(m.entries, 0, len(m.entries))
	m.clears++
}

// LoadOrStore returns the existing value for key if present. Otherwise, it
// stores value as Store does and returns it. The loaded result is true if
// the value was loaded, false if stored. The lookup and the store
// happen atomically.
func (m *SyncMap[Key, Value]) LoadOrStore(key Key, value Value) (actual Value, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if actual, loaded = m.load(key); loaded {
		return actual, true
	}
	m.store(key, value)
	return value, false
}

// LoadAndDelete deletes the entry for key, returning its value if any. The
// loaded result reports whether key was present. The lookup and the
// delete happen atomically.
func (m *SyncMap[Key, Value]) LoadAndDelete(key Key) (value Value, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if value, loaded = m.load(key); loaded {
		m.delete(key)
		return value, true
	}
	var zero Value
	return zero, false
}

// Delete deletes the entry for key. It is a no-op if key is not present. The
// remaining entries keep their relative order. Delete takes time
// proportional to the number of entries in m.
func (m *SyncMap[Key, Value]) Delete(key Key) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.delete(key)
}

func (m *SyncMap[Key, Value]) delete(key Key) {
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
// a missing key. The lookup and the replacement happen atomically.
func (m *SyncMap[Key, Value]) Swap(key Key, value Value) (previous Value, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	previous, loaded = m.load(key)
	if !loaded {
		var zero Value
		return zero, false
	}
	m.store(key, value)
	return previous, true
}

// CompareAndSwap stores value for key if key is present and comp(old, value)
// returns true, where old is the value currently stored for key. It reports
// whether the value was swapped. comp is not called if key is not present,
// and is called at most once otherwise. The lock on m is held while comp runs, so comp must not call any
// method of m; doing so deadlocks.
func (m *SyncMap[Key, Value]) CompareAndSwap(
	key Key,
	value Value,
	comp func(old, value Value) bool,
) (swapped bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if v, ok := m.load(key); ok && comp(v, value) {
		m.store(key, value)
		return true
	}
	return false
}

// CompareAndDelete deletes the entry for key if key is present and comp(old)
// returns true, where old is the value currently stored for key. It reports
// whether the entry was deleted. comp is not called if key is not present,
// and is called at most once otherwise. The lock on m is held while comp runs, so comp must not call any
// method of m; doing so deadlocks.
func (m *SyncMap[Key, Value]) CompareAndDelete(
	key Key,
	comp func(old Value) bool,
) (deleted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if v, ok := m.load(key); ok && comp(v) {
		m.delete(key)
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
// f is called without holding the lock on m, so f may call any method of m.
// Range does not correspond to any consistent snapshot of the map's
// contents: if entries are stored or deleted concurrently, including by f,
// the rules above apply to those changes as well.
//
// Range copies the list of entries before the first call to f, so it takes
// time and memory proportional to the number of entries even if f returns
// false after a constant number of calls.
func (m *SyncMap[Key, Value]) Range(f func(key Key, value Value) bool) {
	m.mu.RLock()
	entries := slices.Clone(m.entries)
	clears := m.clears
	m.mu.RUnlock()

	for _, e := range entries {
		m.mu.RLock()
		if _, ok := m.idx[e.key]; !ok {
			// nolint:gocritic // a NaN key wouldn't satisfy e.key == e.key
			if e.key == e.key {
				m.mu.RUnlock()
				continue
			}
			if clears != m.clears {
				m.mu.RUnlock()
				return
			}
		}

		cur := e
		// nolint:gocritic // a NaN key wouldn't satisfy e.key == e.key
		if e.key == e.key {
			cur = m.entries[m.idx[e.key]]
		}
		m.mu.RUnlock()
		if !f(cur.key, cur.value) {
			return
		}
	}
}

// All returns an iterator over the key-value pairs in m, in insertion order.
// Each use of the iterator behaves as a call to [SyncMap.Range].
func (m *SyncMap[Key, Value]) All() iter.Seq2[Key, Value] {
	return func(yield func(Key, Value) bool) {
		m.Range(yield)
	}
}

// Keys returns an iterator over the keys in m, in insertion order. Each use
// of the iterator behaves as a call to [SyncMap.Range].
func (m *SyncMap[Key, Value]) Keys() iter.Seq[Key] {
	return func(yield func(Key) bool) {
		m.Range(func(key Key, _ Value) bool {
			return yield(key)
		})
	}
}

// Values returns an iterator over the values in m, in the insertion order of
// their keys. Each use of the iterator behaves as a call to [SyncMap.Range].
func (m *SyncMap[Key, Value]) Values() iter.Seq[Value] {
	return func(yield func(Value) bool) {
		m.Range(func(_ Key, value Value) bool {
			return yield(value)
		})
	}
}

// Clone returns a new SyncMap with the same entries in the same order. It is a
// shallow clone: keys and values are copied using ordinary assignment. The
// clone and m are independent; changes to one do not affect the other. m is
// locked for reading while it is cloned.
func (m *SyncMap[K, V]) Clone() *SyncMap[K, V] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &SyncMap[K, V]{
		idx:     maps.Clone(m.idx),
		entries: slices.Clone(m.entries),
	}
}
