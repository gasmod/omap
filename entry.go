package omap

// entry is a key-value pair stored in a map's ordered list of entries.
type entry[Key comparable, Value any] struct {
	key   Key
	value Value
	// seq is the entry's insertion sequence number, which increases along
	// the list. A deleted entry keeps its number with deadBit set.
	seq uint64
}

// deadBit marks a deleted entry's seq.
const deadBit = 1 << 63

func (e *entry[Key, Value]) dead() bool {
	return e.seq&deadBit != 0
}
