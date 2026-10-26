package omap

import (
	"maps"
	"slices"
	"sort"
)

// core holds the state and logic Map and SyncMap share. It is not safe for
// concurrent use; SyncMap guards it with a lock.
//
// Entries are kept in a list in insertion order. Delete does not shift the
// list: it marks the entry dead, and the dead entries are dropped when they
// reach the end of the list or make up more than half of it. idx maps each
// live key to its position in the list. It also holds an unreachable entry
// for each NaN key, so len(idx) counts every live entry.
type core[Key comparable, Value any] struct {
	idx     map[Key]int
	entries []entry[Key, Value]
	// seq is the sequence number of the most recently stored entry.
	seq uint64
	// dead is the number of dead entries in entries.
	dead int
	// gen changes whenever entries move to new positions, so that cursors
	// know to find their place again.
	gen uint64
}

func (c *core[Key, Value]) len() int {
	return len(c.idx)
}

func (c *core[Key, Value]) has(key Key) bool {
	_, ok := c.idx[key]
	return ok
}

func (c *core[Key, Value]) load(key Key) (Value, bool) {
	if i, ok := c.idx[key]; ok {
		return c.entries[i].value, true
	}
	var zero Value
	return zero, false
}

func (c *core[Key, Value]) store(key Key, value Value) {
	if i, ok := c.idx[key]; ok {
		c.entries[i].key = key // for -0 == +0
		c.entries[i].value = value
		return
	}
	c.add(key, value)
}

// add appends a new entry for key, which must not be present.
func (c *core[Key, Value]) add(key Key, value Value) {
	if c.idx == nil {
		c.idx = make(map[Key]int)
	}
	c.seq++
	c.entries = append(c.entries, entry[Key, Value]{key: key, value: value, seq: c.seq})
	c.idx[key] = len(c.entries) - 1
}

func (c *core[Key, Value]) loadOrStore(key Key, value Value) (Value, bool) {
	if i, ok := c.idx[key]; ok {
		return c.entries[i].value, true
	}
	c.add(key, value)
	return value, false
}

func (c *core[Key, Value]) loadAndDelete(key Key) (Value, bool) {
	if i, ok := c.idx[key]; ok {
		value := c.entries[i].value
		c.deleteAt(key, i)
		return value, true
	}
	var zero Value
	return zero, false
}

func (c *core[Key, Value]) delete(key Key) {
	if i, ok := c.idx[key]; ok {
		c.deleteAt(key, i)
	}
}

func (c *core[Key, Value]) swap(key Key, value Value) (Value, bool) {
	i, ok := c.idx[key]
	if !ok {
		var zero Value
		return zero, false
	}
	previous := c.entries[i].value
	c.entries[i].key = key // for -0 == +0
	c.entries[i].value = value
	return previous, true
}

// deleteAt deletes key, which is stored at position i.
func (c *core[Key, Value]) deleteAt(key Key, i int) {
	delete(c.idx, key)
	// Clear the key and value so they can be garbage collected.
	c.entries[i] = entry[Key, Value]{seq: c.entries[i].seq | deadBit}
	c.dead++

	// Dropping dead entries from the end moves no live entry.
	n := len(c.entries)
	for n > 0 && c.entries[n-1].dead() {
		n--
	}
	c.dead -= len(c.entries) - n
	c.entries = c.entries[:n]

	if c.dead > len(c.entries)/2 {
		c.compact()
	}
}

// compact drops the dead entries, moving the live ones down.
func (c *core[Key, Value]) compact() {
	j := 0
	for i := range c.entries {
		e := c.entries[i]
		if e.dead() {
			continue
		}
		if i != j {
			c.entries[j] = e
			// A NaN key can't be looked up, and storing it would add an
			// entry to idx.
			// nolint:gocritic // a NaN key wouldn't satisfy e.key == e.key
			if e.key == e.key {
				c.idx[e.key] = j
			}
		}
		j++
	}
	clear(c.entries[j:])
	c.entries = c.entries[:j]
	c.dead = 0
	c.gen++
}

func (c *core[Key, Value]) clear() {
	// Clearing a builtin map takes time proportional to its capacity, so idx
	// is only reused while it holds a fair share of the entries it has room
	// for; the capacity of entries tracks how large the map has been.
	if len(c.idx) >= cap(c.entries)/4 {
		clear(c.idx)
	} else {
		c.idx = make(map[Key]int)
	}
	clear(c.entries)
	c.entries = c.entries[:0]
	c.dead = 0
	c.gen++
}

func (c *core[Key, Value]) clone() core[Key, Value] {
	return core[Key, Value]{
		idx:     maps.Clone(c.idx),
		entries: slices.Clone(c.entries),
		seq:     c.seq,
		dead:    c.dead,
	}
}

// cursor is the position of an iteration over a core.
type cursor struct {
	// i is the position of the next entry to consider.
	i int
	// last is the sequence number of the last entry produced, or 0.
	last uint64
	// end is the sequence number of the last entry stored before the
	// iteration began. Later entries are not produced, so a key that is
	// deleted and stored again is never produced twice.
	end uint64
	gen uint64
}

func (c *core[Key, Value]) cursor() cursor {
	return cursor{end: c.seq, gen: c.gen}
}

// next returns the next live entry for cur and advances it. ok is false
// when the iteration is done.
func (c *core[Key, Value]) next(cur *cursor) (key Key, value Value, ok bool) {
	if cur.gen != c.gen {
		// Entries have moved since cur was last advanced. The list is
		// ordered by sequence number, so resume after the last one produced.
		cur.gen = c.gen
		cur.i = sort.Search(len(c.entries), func(i int) bool {
			return c.entries[i].seq&^deadBit > cur.last
		})
	}
	for cur.i < len(c.entries) {
		e := &c.entries[cur.i]
		if e.seq&^deadBit > cur.end {
			break
		}
		cur.i++
		if e.dead() {
			continue
		}
		cur.last = e.seq
		return e.key, e.value, true
	}
	return key, value, false
}
