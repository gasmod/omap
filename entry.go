package omap

// entry is a key-value pair stored in a map's ordered list of entries.
type entry[Key comparable, Value any] struct {
	key   Key
	value Value
}
