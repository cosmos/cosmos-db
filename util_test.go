package db

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Empty iterator for empty db.
func TestPrefixIteratorNoMatchNil(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Prefix w/ backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)
			itr, err := IteratePrefix(db, []byte("2"))
			require.NoError(t, err)

			checkInvalid(t, itr)
		})
	}
}

// Empty iterator for db populated after iterator created.
func TestPrefixIteratorNoMatch1(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Prefix w/ backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)
			itr, err := IteratePrefix(db, []byte("2"))
			require.NoError(t, err)
			err = db.SetSync(stringToBytes("1"), stringToBytes("value_1"))
			require.NoError(t, err)

			checkInvalid(t, itr)
		})
	}
}

// Empty iterator for prefix starting after db entry.
func TestPrefixIteratorNoMatch2(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Prefix w/ backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)
			err := db.SetSync(stringToBytes("3"), stringToBytes("value_3"))
			require.NoError(t, err)
			itr, err := IteratePrefix(db, []byte("4"))
			require.NoError(t, err)

			checkInvalid(t, itr)
		})
	}
}

// Iterator with single val for db with single val, starting from that val.
func TestPrefixIteratorMatch1(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Prefix w/ backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)
			err := db.SetSync(stringToBytes("2"), stringToBytes("value_2"))
			require.NoError(t, err)
			itr, err := IteratePrefix(db, stringToBytes("2"))
			require.NoError(t, err)

			checkValid(t, itr, true)
			checkItem(t, itr, stringToBytes("2"), stringToBytes("value_2"))
			checkNext(t, itr, false)

			// Once invalid...
			checkInvalid(t, itr)
		})
	}
}

// Iterator with prefix iterates over everything with same prefix.
func TestPrefixIteratorMatches1N(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Prefix w/ backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)

			// prefixed
			err := db.SetSync(stringToBytes("a/1"), stringToBytes("value_1"))
			require.NoError(t, err)
			err = db.SetSync(stringToBytes("a/3"), stringToBytes("value_3"))
			require.NoError(t, err)

			// not
			err = db.SetSync(stringToBytes("b/3"), stringToBytes("value_3"))
			require.NoError(t, err)
			err = db.SetSync(stringToBytes("a-3"), stringToBytes("value_3"))
			require.NoError(t, err)
			err = db.SetSync(stringToBytes("a.3"), stringToBytes("value_3"))
			require.NoError(t, err)
			err = db.SetSync(stringToBytes("abcdefg"), stringToBytes("value_3"))
			require.NoError(t, err)
			itr, err := IteratePrefix(db, stringToBytes("a/"))
			require.NoError(t, err)

			checkValid(t, itr, true)
			checkItem(t, itr, stringToBytes("a/1"), stringToBytes("value_1"))
			checkNext(t, itr, true)
			checkItem(t, itr, stringToBytes("a/3"), stringToBytes("value_3"))

			// Bad!
			checkNext(t, itr, false)

			// Once invalid...
			checkInvalid(t, itr)
		})
	}
}
