package db

import (
	"bytes"

	"github.com/tidwall/btree"
)

const (
	// Size of the channel buffer between traversal goroutine and iterator. Using an unbuffered
	// channel causes two context switches per item sent, while buffering allows more work per
	// context switch. Tuned with benchmarks.
	chBufferSize = 64
)

// memDBIterator is a memDB iterator.
type memDBIterator struct {
	iter btree.IterG[item]

	start     []byte
	end       []byte
	ascending bool
	valid     bool
}

var _ Iterator = (*memDBIterator)(nil)

// newMemDBIterator creates a new memDBIterator.
func newMemDBIterator(start, end []byte, items MemDB, ascending bool) *memDBIterator {
	iter := items.btree.Iter()
	var valid bool
	if ascending {
		if start != nil {
			valid = iter.Seek(newItem(start, nil))
		} else {
			valid = iter.First()
		}
	} else {
		if end != nil {
			valid = iter.Seek(newItem(end, nil))
			if !valid {
				valid = iter.Last()
			} else {
				// end is exclusive
				valid = iter.Prev()
			}
		} else {
			valid = iter.Last()
		}
	}

	mi := &memDBIterator{
		iter:      iter,
		start:     start,
		end:       end,
		ascending: ascending,
		valid:     valid,
	}

	if mi.valid {
		mi.valid = mi.keyInRange(mi.Key())
	}

	return mi
}

func (mi *memDBIterator) keyInRange(key []byte) bool {
	if mi.ascending && mi.end != nil && bytes.Compare(key, mi.end) >= 0 {
		return false
	}
	if !mi.ascending && mi.start != nil && bytes.Compare(key, mi.start) < 0 {
		return false
	}
	return true
}

// Close implements Iterator.
func (i *memDBIterator) Close() error {

	i.iter.Release()

	return nil
}

// Domain implements Iterator.
func (i *memDBIterator) Domain() ([]byte, []byte) {
	return i.start, i.end
}

// Valid implements Iterator.
func (i *memDBIterator) Valid() bool {
	return i.valid
}

// Next implements Iterator.
func (mi *memDBIterator) Next() {
	mi.assertValid()

	if mi.ascending {
		mi.valid = mi.iter.Next()
	} else {
		mi.valid = mi.iter.Prev()
	}

	if mi.valid {
		mi.valid = mi.keyInRange(mi.Key())
	}
}

func (mi *memDBIterator) assertValid() {
	if err := mi.Error(); err != nil {
		panic(err)
	}
}

// Error implements Iterator.
func (i *memDBIterator) Error() error {
	return nil // famous last words
}

// Key implements Iterator.
func (i *memDBIterator) Key() []byte {
	i.assertIsValid()
	return i.iter.Item().key
}

// Value implements Iterator.
func (i *memDBIterator) Value() []byte {
	i.assertIsValid()
	return i.iter.Item().value
}

func (i *memDBIterator) assertIsValid() {
	if !i.Valid() {
		panic("iterator is invalid")
	}
}
