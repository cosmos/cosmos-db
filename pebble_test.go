package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPebbleBackgroundErrorIsLogged: a failed compaction or flush reports
// only through this handler, and EnsureDefaults builds it from our Logger --
// so a level that Logger drops loses the report and the database goes on
// looking healthy. v1 defaulted to the Infof it drops; v2 uses Errorf.
func TestPebbleBackgroundErrorIsLogged(t *testing.T) {
	buf := captureLog(t)

	pebbleOptions().EventListener.BackgroundError(errors.New("boom"))
	require.Contains(t, buf.String(), "boom")
}

// TestPebbleInfoIsDropped is the other half, and the reason fatalLogger
// exists: pebble's info logging drowns the node's own.
func TestPebbleInfoIsDropped(t *testing.T) {
	buf := captureLog(t)

	pebbleOptions().Logger.Infof("compacted %d tables", 3)
	require.Empty(t, buf.String())
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

// TestPebbleCorruptTableIsFatal covers a table pebble cannot fully read. On
// v1 a compaction over one finished and reported success, leaving an output
// missing exactly the keys it could not read and nothing in the log -- the
// database came out smaller and called itself healthy. v2 ends the process
// instead, which is the outcome we want: a stopped database is recoverable
// from a snapshot or a peer, one quietly missing keys is not.
//
// What is covered here is ours, not pebble's: ending the process is what
// pebbleOptions leaves to the DataCorruption default, so giving that a
// handler of our own -- logging the damage and carrying on, say -- puts the
// database back to serving past blocks it cannot read. This is what says so.
func TestPebbleCorruptTableIsFatal(t *testing.T) {
	// The corrupt cases end in os.Exit, so each runs as a child process and
	// the parent inspects how it died.
	if dir := os.Getenv("PEBBLE_CORRUPTION_CHILD"); dir != "" {
		runCorruptionChild(t, dir, os.Getenv("PEBBLE_CORRUPTION_STEP"))
		return
	}

	// The control: the same scan and compaction over an intact table.
	t.Run("healthy", func(t *testing.T) {
		dir := t.TempDir()
		writeOverlappingKeys(t, dir)
		buf := captureLog(t)

		db, err := NewPebbleDB("application", dir, nil)
		require.NoError(t, err)
		defer db.Close()

		before, err := countKeys(t, db)
		require.NoError(t, err)
		require.Equal(t, corruptionKeys, before)

		require.NoError(t, db.(*PebbleDB).db.Compact(
			context.Background(), []byte("key"), []byte("kez"), true))

		after, err := countKeys(t, db)
		require.NoError(t, err)
		require.Equal(t, corruptionKeys, after, "an intact table must lose nothing")
		require.Empty(t, buf.String(), "an intact table must report nothing")
	})

	// scan reaches the damage the way a query does; compact reaches it the way
	// pebble's own background work does, without a read having gone first.
	for _, step := range []string{"scan", "compact"} {
		t.Run(step, func(t *testing.T) {
			// Built in the parent so it outlives the child and can be
			// inspected after the crash.
			dir := t.TempDir()
			writeOverlappingKeys(t, dir)
			table := corruptLargestSST(t, filepath.Join(dir, "application"+DBFileSuffix))

			// -test.v so the child's logs are written as they happen: os.Exit
			// leaves nothing to flush them at the end.
			cmd := exec.Command(os.Args[0], "-test.run=TestPebbleCorruptTableIsFatal", "-test.v")
			cmd.Env = append(os.Environ(),
				"PEBBLE_CORRUPTION_CHILD="+dir, "PEBBLE_CORRUPTION_STEP="+step)
			out, err := cmd.CombinedOutput()
			t.Logf("child exited with %v:\n%s", err, out)

			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr, "the child must not survive the corruption")
			require.Contains(t, string(out), "on-disk corruption",
				"the corruption must be named in what the child printed on its way out")
			require.NotContains(t, string(out), childSurvived,
				"pebble must not report the damaged table as read or compacted")

			// The v1 failure was a compaction consuming the damaged table and
			// leaving an output without its keys; the table still being here
			// is that not having happened.
			require.FileExists(t, table,
				"the damaged table must not have been rewritten without its unreadable block")
		})
	}
}

const childSurvived = "survived:"

// runCorruptionChild runs in the child process: it opens the damaged database
// and reaches the corruption the way `step` says. Production compacts on its
// own schedule; forcing it here only makes the timing predictable.
func runCorruptionChild(t *testing.T, dir, step string) {
	db, err := NewPebbleDB("application", dir, nil)
	require.NoError(t, err)
	defer db.Close()

	switch step {
	case "scan":
		n, err := countKeys(t, db)
		t.Logf("%s scan of %d keys, err=%v", childSurvived, n, err)
	case "compact":
		err := db.(*PebbleDB).db.Compact(
			context.Background(), []byte("key"), []byte("kez"), true)
		t.Logf("%s compaction, err=%v", childSurvived, err)
	default:
		t.Fatalf("unknown step %q", step)
	}
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

// syncBuf collects log output, which pebble also writes from its background
// goroutines.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureLog redirects the standard logger, which is where pebble's own
// DefaultLogger writes, for the duration of the test.
func captureLog(t *testing.T) *syncBuf {
	t.Helper()
	buf := &syncBuf{}
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return buf
}

// corruptLargestSST overwrites bytes a little way into the biggest sstable,
// past its header but well before the index and footer, so the file still
// opens and the damage surfaces as a block checksum failure on read. It
// returns the table's path.
func corruptLargestSST(t *testing.T, dbDir string) string {
	t.Helper()
	tables, err := filepath.Glob(filepath.Join(dbDir, "*.sst"))
	require.NoError(t, err)
	require.NotEmpty(t, tables, "no sstables were written; nothing to corrupt")
	sort.Slice(tables, func(i, j int) bool {
		a, _ := os.Stat(tables[i])
		b, _ := os.Stat(tables[j])
		return a.Size() > b.Size()
	})

	target := tables[0]
	info, err := os.Stat(target)
	require.NoError(t, err)
	f, err := os.OpenFile(target, os.O_RDWR, 0)
	require.NoError(t, err)
	defer f.Close()

	_, err = f.WriteAt(bytes.Repeat([]byte{0xA5}, 4096), info.Size()/4)
	require.NoError(t, err)
	t.Logf("corrupted %s (%d bytes)", filepath.Base(target), info.Size())
	return target
}

const corruptionKeys = 40000

// writeOverlappingKeys fills a database with tables that all span the whole
// keyspace, so a compaction has to read them rather than move them.
func writeOverlappingKeys(t *testing.T, dir string) {
	t.Helper()
	db, err := NewPebbleDB("application", dir, nil)
	require.NoError(t, err)
	// A stride coprime with the count, so each memtable spans the whole
	// keyspace. Written in order the resulting tables would not overlap and
	// compaction would move them between levels without reading them, which
	// is no test of what happens when a read goes wrong.
	value := make([]byte, 512)
	for i := 0; i < corruptionKeys; i++ {
		key := fmt.Sprintf("key%08d", (i*104729)%corruptionKeys)
		require.NoError(t, db.Set([]byte(key), value))
	}
	require.NoError(t, db.Close())
}

func countKeys(t *testing.T, db DB) (int, error) {
	t.Helper()
	it, err := db.Iterator(nil, nil)
	require.NoError(t, err)
	defer it.Close()
	n := 0
	for ; it.Valid(); it.Next() {
		n++
	}
	return n, it.Error()
}
