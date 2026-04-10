package db

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestGoLevelDBNewGoLevelDB(t *testing.T) {
	name := fmt.Sprintf("test_%x", randStr(12))
	defer cleanupDBDir("", name)

	// Test we can't open the db twice for writing
	wr1, err := NewGoLevelDB(name, "", nil)
	require.Nil(t, err)
	_, err = NewGoLevelDB(name, "", nil)
	require.NotNil(t, err)
	require.NoError(t, wr1.Close()) // Close the db to release the lock

	// Test we can open the db twice for reading only
	ro1, err := NewGoLevelDBWithOpts(name, "", &opt.Options{ReadOnly: true})
	require.Nil(t, err)
	defer ro1.Close()
	ro2, err := NewGoLevelDBWithOpts(name, "", &opt.Options{ReadOnly: true})
	require.Nil(t, err)
	defer ro2.Close()
}

func TestGoLevelDBEmptyValue(t *testing.T) {
	name := fmt.Sprintf("test_%x", randStr(12))
	defer cleanupDBDir("", name)

	db, err := NewGoLevelDB(name, "", nil)
	require.NoError(t, err)
	defer db.Close()

	key := []byte("mykey")

	// Set a key with an empty value
	err = db.Set(key, []byte{})
	require.NoError(t, err)

	// Has should return true for a key with an empty value
	has, err := db.Has(key)
	require.NoError(t, err)
	require.True(t, has, "Has() should return true for a key with an empty value")

	// Get should return non-nil (empty slice) for a key with an empty value
	val, err := db.Get(key)
	require.NoError(t, err)
	require.NotNil(t, val, "Get() should return non-nil for a key with an empty value")
	require.Empty(t, val)

	// Verify non-existent key still returns nil
	val2, err := db.Get([]byte("nonexistent"))
	require.NoError(t, err)
	require.Nil(t, val2, "Get() should return nil for a non-existent key")

	has2, err := db.Has([]byte("nonexistent"))
	require.NoError(t, err)
	require.False(t, has2)
}

func BenchmarkGoLevelDBRandomReadsWrites(b *testing.B) {
	name := fmt.Sprintf("test_%x", randStr(12))
	db, err := NewGoLevelDB(name, "", nil)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		require.NoError(b, db.Close())
		cleanupDBDir("", name)
	}()

	benchmarkRandomReadsWrites(b, db)
}
