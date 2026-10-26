# omap

Package `omap` provides generic ordered maps for Go: maps that remember the
order in which keys were first stored and produce their entries in that order.

- `Map` is for use by a single goroutine, like a builtin map.
- `SyncMap` is safe for concurrent use by multiple goroutines.

Both have the same method set, which follows `sync.Map` and adds `Len`, `Has`,
`Clone` and the range-over-func iterators `All`, `Keys` and `Values`. The zero
value of either type is an empty map, ready to use.

## Install

```sh
go get github.com/gasmod/omap
```

The module requires Go 1.27.3 or later.

## Usage

```go
var m omap.Map[string, int]
m.Store("banana", 3)
m.Store("apple", 1)
m.Store("cherry", 7)
m.Store("banana", 5) // overwriting keeps the original position

for k, v := range m.All() {
  fmt.Println(k, v)
}
// banana 5
// apple 1
// cherry 7

m.Delete("banana")
m.Store("banana", 9) // deleting and storing again moves it to the end

fmt.Println(m.Len())          // 3
fmt.Println(m.Load("banana")) // 9 true
fmt.Println(m.Has("durian"))  // false
for k := range m.Keys() {
  fmt.Print(k, " ")
}
// apple cherry banana
```

`SyncMap` is used the same way from any number of goroutines. Each method is
atomic, so read-modify-write loops work as they do with `sync.Map`:

```go
var m omap.SyncMap[string, int]
m.Store("hits", 0)

var wg sync.WaitGroup
for range 8 {
  wg.Go(func() {
    for {
      cur, _ := m.Load("hits")
      if m.CompareAndSwap("hits", cur+1, func(old, _ int) bool { return old == cur }) {
        return
      }
    }
  })
}
wg.Wait()

fmt.Println(m.Load("hits")) // 8 true
```

## API

| Method                           | Description                                                                    |
| -------------------------------- | ------------------------------------------------------------------------------ |
| `Len()`                          | Number of entries.                                                             |
| `Has(key)`                       | Whether `key` is present.                                                      |
| `Load(key)`                      | The value for `key`, and whether it was present.                               |
| `Store(key, value)`              | Sets the value. A new key goes to the end; an existing key keeps its position. |
| `LoadOrStore(key, value)`        | Returns the existing value, or stores and returns `value`.                     |
| `LoadAndDelete(key)`             | Deletes `key`, returning its value if it was present.                          |
| `Delete(key)`                    | Deletes `key`. A no-op if it is missing.                                       |
| `Swap(key, value)`               | Replaces the value of a present key and returns the previous one.              |
| `CompareAndSwap(key, new, comp)` | Stores `new` if `key` is present and `comp(old, new)` returns true.            |
| `CompareAndDelete(key, comp)`    | Deletes `key` if it is present and `comp(old)` returns true.                   |
| `Clear()`                        | Deletes every entry.                                                           |
| `Range(f)`                       | Calls `f` for each entry in order until `f` returns false.                     |
| `All()`, `Keys()`, `Values()`    | Iterators over entries, keys or values, in order.                              |
| `Clone()`                        | An independent shallow copy.                                                   |

The full contract of each method is in the
[package documentation](https://pkg.go.dev/github.com/gasmod/omap).

## Semantics

### Order

Entries are kept in insertion order. Storing a value under a key that is
already present replaces the value but keeps the entry where it is. Deleting a
key and storing it again moves it to the end.

### Changing the map while iterating

Iteration follows the rules of ranging over a builtin map, so the function
passed to `Range` (or the body of a `for range` over `All`, `Keys` or
`Values`) may modify the map:

- An entry deleted before iteration reaches it is not produced.
- An entry added during iteration, including a key deleted and stored again,
  may be produced or skipped.
- Each value produced is the value stored for its key at that moment, and no
  key is produced twice.

`SyncMap.Range` calls `f` without holding the lock, so `f` may call any method
of the map, and other goroutines may change it in the meantime.

### Key equality

Keys are compared with `==`, as in a builtin map, and the special cases of
builtin maps carry over:

- NaN is not equal to itself, so every `Store` with a NaN key adds a new entry.
  These entries are counted by `Len` and produced by iteration, but no lookup
  can find them. Only `Clear` removes them.
- `+0.0` and `-0.0` are the same key. Storing one when the other is present
  replaces both the value and the stored key.

## Differences from sync.Map

- `Swap` only replaces the value of a key that is already present. Unlike
  `sync.Map.Swap`, it does not store a missing key.
- `CompareAndSwap` and `CompareAndDelete` take a comparison function instead of
  an expected value, so the value type does not need to be comparable.
- `SyncMap` guards all its state with a single `sync.RWMutex`. Methods that only
  read the map run in parallel, and methods that modify it run one at a time. It
  is not tuned for the write-once, read-many workloads `sync.Map` is designed
  for.
- `SyncMap.CompareAndSwap` and `SyncMap.CompareAndDelete` run `comp` while
  holding the lock, so `comp` must not call methods of the same map; doing so
  deadlocks.

## Copying

Neither type may be copied after first use. Use `Clone` to copy a map.
`go vet` reports copies of either type: `SyncMap` contains a mutex, and `Map`
embeds a marker that vet's copylocks check recognizes.

## Performance

| Operation                                | Cost                                                                                                                                       |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `Len`, `Has`, `Load`                     | Constant time.                                                                                                                             |
| `Store`                                  | Amortized constant time.                                                                                                                   |
| `Delete` and the methods that may delete | Proportional to the number of entries, since later entries are shifted and re-indexed.                                                     |
| `Range`, `All`, `Keys`, `Values`         | Each iteration first copies the list of entries, so it takes time and memory proportional to the number of entries even if it stops early. |
| `Clone`                                  | Proportional to the number of entries.                                                                                                     |

If your workload deletes often from large maps, measure before adopting it.

## Development

The tests live in `test/`, and the `Makefile` wraps the usual tasks:

```sh
make test           # run the tests with the race detector
make test-coverage  # write coverage.out and coverage.html
make lint           # run golangci-lint
make fmt            # format with golangci-lint's formatters
```

## License

[MIT](LICENSE)
