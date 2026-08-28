package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/stretchr/testify/require"
)

// TestPebbleRatchetsFormatMajorVersion covers the upgrade this option exists
// for. Opening through NewPebbleDB has to move an existing database forward,
// not just new ones, since every node already has one from before this.
//
// The test goes when the migration does: v2 deleted FormatMostCompatible, so
// none of it compiles there. Delete it rather than repair it -- but carry the
// ceiling assertion over. Staying below FormatNewest is not about the
// migration; it is what leaves a database readable by the release before, and
// it stays true on v2. Dropping the test takes that with it and nothing says
// so.
func TestPebbleRatchetsFormatMajorVersion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "application"+DBFileSuffix)

	// What a node that has never run this code has on disk.
	old, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	require.Equal(t, pebble.FormatMostCompatible, old.FormatMajorVersion(),
		"pebble's own default is the version v2 refuses")
	require.NoError(t, old.Set([]byte("key"), []byte("value"), pebble.Sync))
	require.NoError(t, old.Close())

	db, err := NewPebbleDB("application", dir, nil)
	require.NoError(t, err)
	defer db.Close()

	// FlushableIngest is pebble v2's FormatMinSupported, which is the point.
	// Staying below FormatNewest is the other half: the next version up would
	// not return from the open above until every table had been rewritten.
	vers := db.(*PebbleDB).db.FormatMajorVersion()
	require.Equal(t, pebble.FormatFlushableIngest, vers)
	require.Less(t, vers, pebble.FormatNewest,
		"going further would put a full rewrite in front of the node's first start")

	got, err := db.Get([]byte("key"))
	require.NoError(t, err)
	require.Equal(t, []byte("value"), got, "the upgrade must carry the data over")
}

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
