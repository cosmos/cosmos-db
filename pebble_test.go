package db

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPebbleDBBackend(t *testing.T) {
	name := fmt.Sprintf("test_%x", randStr(12))
	dir := os.TempDir()
	db, err := NewDB(name, PebbleDBBackend, dir)
	require.NoError(t, err)
	defer cleanupDBDir(dir, name)

	_, ok := db.(*PebbleDB)
	require.True(t, ok)
}

// func TestPebbleDBStats(t *testing.T) {
// 	name := fmt.Sprintf("test_%x", randStr(12))
// 	dir := os.TempDir()
// 	db, err := NewDB(name, PebbleDBBackend, dir)
// 	require.NoError(t, err)
// 	defer cleanupDBDir(dir, name)

// 	require.NotEmpty(t, db.Stats())
// }

func BenchmarkPebbleDBRandomReadsWrites(b *testing.B) {
	name := fmt.Sprintf("test_%x", randStr(12))
	dir := os.TempDir()
	db, err := NewDB(name, PebbleDBBackend, dir)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		require.NoError(b, db.Close())
		cleanupDBDir("", name)
	}()

	benchmarkRandomReadsWrites(b, db)
}

// TODO: Add tests for pebble
