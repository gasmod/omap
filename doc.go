// Package omap provides ordered maps: maps that remember the order in which
// keys were first stored and produce their entries in that order.
//
// Two types are provided. [Map] is for use by a single goroutine, like a
// builtin map. [SyncMap] is safe for concurrent use by multiple goroutines.
// Both have the same method set, which follows [sync.Map], plus [Map.Len],
// [Map.Has], [Map.Clone] and the iterators [Map.All], [Map.Keys] and
// [Map.Values].
//
// # Order
//
// Entries are kept in insertion order. Storing a value under a key that is
// already present replaces the value but keeps the entry in its original
// position. Deleting a key and storing it again moves it to the end.
//
// # Key equality
//
// Keys are compared with ==, exactly as for a builtin map, and the special
// cases of builtin maps carry over:
//
//   - A floating-point NaN is not equal to itself, so every Store with a NaN
//     key adds a new entry. Such entries are counted by Len and produced by
//     iteration, but they can never be found by Load, Has, Delete or any
//     other lookup. Only Clear removes them.
//   - Keys that are equal but distinguishable, such as +0.0 and -0.0, are the
//     same key. Storing one when the other is present replaces both the value
//     and the stored key, so iteration produces the key most recently stored.
//
// # Differences from sync.Map
//
// Swap only replaces the value of a key that is already present; it does not
// store a missing key. CompareAndSwap and CompareAndDelete take a comparison
// function instead of an expected value, so Value need not be comparable.
//
// # Complexity
//
// Len, Has, Load, Store and Delete, and the methods built on them, take
// constant (amortized) time. Delete only marks an entry as deleted; deleted
// entries are dropped once they reach the end of the list or make up half of
// it. Iteration walks the list in place, without copying it.
package omap
