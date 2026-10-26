package omap

import (
	"iter"
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
	core core[Key, Value]
	mu   sync.RWMutex
}

// Len returns the number of entries in m, including entries with NaN keys.
func (m *SyncMap[Key, Value]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.core.len()
}

// Has reports whether key is present in m. It always reports false for a NaN
// key.
func (m *SyncMap[Key, Value]) Has(key Key) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.core.has(key)
}

// Load returns the value stored in m for key, or the zero value if no value
// is present. The ok result reports whether a value was found.
func (m *SyncMap[Key, Value]) Load(key Key) (value Value, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.core.load(key)
}

// Store sets the value for key. If key is not present, a new entry is added
// at the end of m. If key is present, its entry keeps its position and both
// its value and its key are replaced, so a later iteration produces the key
// passed to the most recent Store (for example -0.0 after +0.0).
func (m *SyncMap[Key, Value]) Store(key Key, value Value) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.core.store(key, value)
}

// Clear deletes all the entries, including entries with NaN keys, resulting
// in an empty SyncMap. The storage used for the entries is kept for reuse.
func (m *SyncMap[Key, Value]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.core.clear()
}

// LoadOrStore returns the existing value for key if present. Otherwise, it
// stores value as Store does and returns it. The loaded result is true if
// the value was loaded, false if stored. The lookup and the store
// happen atomically.
func (m *SyncMap[Key, Value]) LoadOrStore(key Key, value Value) (actual Value, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.core.loadOrStore(key, value)
}

// LoadAndDelete deletes the entry for key, returning its value if any. The
// loaded result reports whether key was present. The lookup and the
// delete happen atomically.
func (m *SyncMap[Key, Value]) LoadAndDelete(key Key) (value Value, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.core.loadAndDelete(key)
}

// Delete deletes the entry for key. It is a no-op if key is not present. The
// remaining entries keep their relative order. Delete takes amortized
// constant time.
func (m *SyncMap[Key, Value]) Delete(key Key) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.core.delete(key)
}

// Swap replaces the value for key if key is present and returns the previous
// value with loaded set to true. The entry keeps its position and its key is
// replaced as in Store. If key is not present, m is unchanged and Swap
// returns the zero value and false; unlike [sync.Map.Swap], it does not store
// a missing key. The lookup and the replacement happen atomically.
func (m *SyncMap[Key, Value]) Swap(key Key, value Value) (previous Value, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.core.swap(key, value)
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

	if v, ok := m.core.load(key); ok && comp(v, value) {
		m.core.store(key, value)
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

	if v, ok := m.core.load(key); ok && comp(v) {
		m.core.delete(key)
		return true
	}
	return false
}

// Range calls f sequentially for each key and value in m, in insertion
// order. If f returns false, Range stops the iteration.
//
// The iteration follows the rules of ranging over a builtin map, with one
// stricter guarantee: an entry added during the iteration, including a key
// that is deleted and stored again, is never produced. An entry that is
// deleted before it is reached is not produced. Each value produced is the
// value stored for its key at the moment it is produced, and no key is
// produced twice. Entries with NaN keys are produced, unless m is cleared
// before they are reached.
//
// f is called without holding the lock on m, so f may call any method of m.
// Range does not correspond to any consistent snapshot of the map's
// contents: if entries are stored or deleted concurrently, including by f,
// the rules above apply to those changes as well.
//
// Range does not copy the list of entries, so it allocates no memory, and
// it stops without visiting the remaining entries when f returns false.
func (m *SyncMap[Key, Value]) Range(f func(key Key, value Value) bool) {
	m.mu.RLock()
	cur := m.core.cursor()
	m.mu.RUnlock()

	for {
		m.mu.RLock()
		key, value, ok := m.core.next(&cur)
		m.mu.RUnlock()
		if !ok || !f(key, value) {
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

	return &SyncMap[K, V]{core: m.core.clone()}
}
