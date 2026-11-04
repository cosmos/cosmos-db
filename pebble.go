package db

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/spf13/cast"
)

// ForceSync
/*
This is set at compile time. Could be 0 or 1, defaults is 0.
It will force using Sync for NoSync functions (Set, Delete, Write)

Used as a workaround for chain-upgrade issue: At the upgrade-block, the sdk will panic without flushing data to disk or
closing dbs properly.

Upgrade guide:
    1. After seeing `UPGRADE "xxxx" NEED at height....`,
       restart current version with
       `-X github.com/tendermint/tm-db.ForceSync=1`
	2. Restart new version as normal


Example: Upgrading sifchain from v0.14.0 to v0.15.0

# log:
panic: UPGRADE "0.15.0" NEEDED at height: 8170210: {
    "binaries": {
        "linux/amd64": "https://github.com/Sifchain/sifnode/releases/download/v0.15.0/" +
        "sifnoded-v0.15.0-linux-amd64.zip?checksum=0c03b5846c5a13dcc0d9d3127e4f0cee0aeddcf2165177b2f2e0d60dbcf1a5ea"
    }
}

# step1
git reset --hard
git checkout v0.14.0
go mod edit -replace github.com/tendermint/tm-db=github.com/baabeetaa/tm-db@pebble
go mod tidy
go install -ldflags "-w -s \
    -X github.com/cosmos/cosmos-sdk/types.DBBackend=pebbledb \
    -X github.com/tendermint/tm-db.ForceSync=1" ./cmd/sifnoded

$HOME/go/bin/sifnoded start --db_backend=pebbledb


# step 2
git reset --hard
git checkout v0.15.0
go mod edit -replace github.com/tendermint/tm-db=github.com/baabeetaa/tm-db@pebble
go mod tidy
go install -ldflags "-w -s -X github.com/cosmos/cosmos-sdk/types.DBBackend=pebbledb" ./cmd/sifnoded

$HOME/go/bin/sifnoded start --db_backend=pebbledb

*/
var (
	ForceSync   = "0"
	isForceSync = false
)

func init() {
	registerDBCreator(PebbleDBBackend, NewPebbleDB, false)

	if ForceSync == "1" {
		isForceSync = true
	}
}

// PebbleDB is a PebbleDB backend.
//
// All operations hold a read lock, while Close holds a write lock to prevent
// closing Pebble during an in-flight operation. Calls after Close return
// pebble.ErrClosed.
type PebbleDB struct {
	db *pebble.DB

	mu     sync.RWMutex
	closed bool

	// Close releases all open iterators before closing the DB.
	itersMu sync.Mutex
	iters   map[*pebbleDBIterator]struct{}
}

var _ DB = (*PebbleDB)(nil)

// NewPebbleDB opens dir/name.db with cosmos-db's defaults. opts can set
// "maxopenfiles".
func NewPebbleDB(name, dir string, opts Options) (DB, error) {
	o := &pebble.Options{}
	if opts != nil {
		files := cast.ToInt(opts.Get("maxopenfiles"))
		if files > 0 {
			o.MaxOpenFiles = files
		}
	}
	db, err := NewPebbleDBWithOpts(name, dir, o)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// NewPebbleDBWithOpts opens dir/name.db with the supplied Pebble options.
// A nil options value is equivalent to an empty one. Unset fields retain
// Pebble's defaults, except for the logger and concurrent compactions.
//
// If o.Cache is set, the caller retains the reference returned by
// pebble.NewCache and must Unref it when done. The DB takes its own reference
// and releases it on Close.
func NewPebbleDBWithOpts(name, dir string, o *pebble.Options) (*PebbleDB, error) {
	o = o.Clone()
	if o.Logger == nil && o.LoggerAndTracer == nil {
		o.Logger = &fatalLogger{}
	}
	if o.MaxConcurrentCompactions == nil {
		o.MaxConcurrentCompactions = func() int { return 3 }
	}
	o.EnsureDefaults()

	p, err := pebble.Open(filepath.Join(dir, name+DBFileSuffix), o)
	if err != nil {
		return nil, err
	}
	return &PebbleDB{
		db:    p,
		iters: make(map[*pebbleDBIterator]struct{}),
	}, nil
}

// rlock acquires a read lock unless the DB is closed.
func (db *PebbleDB) rlock() error {
	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return pebble.ErrClosed
	}
	return nil
}

// Get implements DB.
func (db *PebbleDB) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errKeyEmpty
	}
	if err := db.rlock(); err != nil {
		return nil, err
	}
	defer db.mu.RUnlock()

	res, closer, err := db.db.Get(key)
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()

	return cp(res), nil
}

// Has implements DB.
func (db *PebbleDB) Has(key []byte) (bool, error) {
	// fmt.Println("PebbleDB.Has")
	if len(key) == 0 {
		return false, errKeyEmpty
	}
	bz, err := db.Get(key)
	if err != nil {
		return false, err
	}
	return bz != nil, nil
}

// Set implements DB.
func (db *PebbleDB) Set(key, value []byte) error {
	// fmt.Println("PebbleDB.Set")
	if len(key) == 0 {
		return errKeyEmpty
	}
	if value == nil {
		return errValueNil
	}

	wopts := pebble.NoSync
	if isForceSync {
		wopts = pebble.Sync
	}
	if err := db.rlock(); err != nil {
		return err
	}
	defer db.mu.RUnlock()

	err := db.db.Set(key, value, wopts)
	if err != nil {
		return err
	}
	return nil
}

// SetSync implements DB.
func (db *PebbleDB) SetSync(key, value []byte) error {
	// fmt.Println("PebbleDB.SetSync")
	if len(key) == 0 {
		return errKeyEmpty
	}
	if value == nil {
		return errValueNil
	}
	if err := db.rlock(); err != nil {
		return err
	}
	defer db.mu.RUnlock()
	err := db.db.Set(key, value, pebble.Sync)
	if err != nil {
		return err
	}
	return nil
}

// Delete implements DB.
func (db *PebbleDB) Delete(key []byte) error {
	// fmt.Println("PebbleDB.Delete")
	if len(key) == 0 {
		return errKeyEmpty
	}

	wopts := pebble.NoSync
	if isForceSync {
		wopts = pebble.Sync
	}
	if err := db.rlock(); err != nil {
		return err
	}
	defer db.mu.RUnlock()
	return db.db.Delete(key, wopts)
}

// DeleteSync implements DB.
func (db *PebbleDB) DeleteSync(key []byte) error {
	// fmt.Println("PebbleDB.DeleteSync")
	if len(key) == 0 {
		return errKeyEmpty
	}
	if err := db.rlock(); err != nil {
		return err
	}
	defer db.mu.RUnlock()
	return db.db.Delete(key, pebble.Sync)
}

// DB returns the underlying Pebble database. Calls on it after Close may panic.
func (db *PebbleDB) DB() *pebble.DB {
	return db.db
}

// Close waits for in-flight operations, closes open iterators, and closes
// Pebble. Subsequent calls return pebble.ErrClosed.
func (db *PebbleDB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return pebble.ErrClosed
	}
	db.closed = true
	for itr := range db.iters {
		_ = itr.source.Close()
		itr.source = nil
		itr.err = pebble.ErrClosed
	}
	db.iters = nil
	return db.db.Close()
}

// Print implements DB.
func (db *PebbleDB) Print() error {
	itr, err := db.Iterator(nil, nil)
	if err != nil {
		return err
	}
	defer itr.Close()
	for ; itr.Valid(); itr.Next() {
		key := itr.Key()
		value := itr.Value()
		fmt.Printf("[%X]:\t[%X]\n", key, value)
	}
	return nil
}

// Stats implements DB.
func (db *PebbleDB) Stats() map[string]string {
	/*
		keys := []string{"rocksdb.stats"}
		stats := make(map[string]string, len(keys))
		for _, key := range keys {
			stats[key] = db.(key)
		}
	*/
	return nil
}

// NewBatch implements DB.
func (db *PebbleDB) NewBatch() Batch {
	return newPebbleDBBatch(db)
}

// NewBatchWithSize implements DB.
// It does the same thing as NewBatch because we can't pre-allocate pebbleDBBatch
func (db *PebbleDB) NewBatchWithSize(_ int) Batch {
	return newPebbleDBBatch(db)
}

// Iterator implements DB.
func (db *PebbleDB) Iterator(start, end []byte) (Iterator, error) {
	// fmt.Println("PebbleDB.Iterator")
	if (start != nil && len(start) == 0) || (end != nil && len(end) == 0) {
		return nil, errKeyEmpty
	}
	o := pebble.IterOptions{
		LowerBound: start,
		UpperBound: end,
	}
	if err := db.rlock(); err != nil {
		return nil, err
	}
	defer db.mu.RUnlock()
	itr, err := db.db.NewIter(&o)
	if err != nil {
		return nil, err
	}
	itr.First()

	return newPebbleDBIterator(db, itr, start, end, false), nil
}

// ReverseIterator implements DB.
func (db *PebbleDB) ReverseIterator(start, end []byte) (Iterator, error) {
	// fmt.Println("PebbleDB.ReverseIterator")
	if (start != nil && len(start) == 0) || (end != nil && len(end) == 0) {
		return nil, errKeyEmpty
	}
	o := pebble.IterOptions{
		LowerBound: start,
		UpperBound: end,
	}
	if err := db.rlock(); err != nil {
		return nil, err
	}
	defer db.mu.RUnlock()
	itr, err := db.db.NewIter(&o)
	if err != nil {
		return nil, err
	}
	itr.Last()
	return newPebbleDBIterator(db, itr, start, end, true), nil
}

var _ Batch = (*pebbleDBBatch)(nil)

type pebbleDBBatch struct {
	db    *PebbleDB
	batch *pebble.Batch
}

var _ Batch = (*pebbleDBBatch)(nil)

// newPebbleDBBatch can be called after Close; Write returns pebble.ErrClosed.
func newPebbleDBBatch(db *PebbleDB) *pebbleDBBatch {
	return &pebbleDBBatch{
		db:    db,
		batch: db.db.NewBatch(),
	}
}

// Set implements Batch.
func (b *pebbleDBBatch) Set(key, value []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	if value == nil {
		return errValueNil
	}
	if b.batch == nil {
		return errBatchClosed
	}
	return b.batch.Set(key, value, nil)
}

// Delete implements Batch.
func (b *pebbleDBBatch) Delete(key []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	if b.batch == nil {
		return errBatchClosed
	}
	return b.batch.Delete(key, nil)
}

// Write implements Batch.
func (b *pebbleDBBatch) Write() error {
	if b.batch == nil {
		return errBatchClosed
	}

	wopts := pebble.NoSync
	if isForceSync {
		wopts = pebble.Sync
	}
	if err := b.db.rlock(); err != nil {
		return err
	}
	defer b.db.mu.RUnlock()
	err := b.batch.Commit(wopts)
	if err != nil {
		return err
	}
	// Make sure batch cannot be used afterwards. Callers should still call Close(), for errors.

	return b.Close()
}

// WriteSync implements Batch.
func (b *pebbleDBBatch) WriteSync() error {
	// fmt.Println("pebbleDBBatch.WriteSync")
	if b.batch == nil {
		return errBatchClosed
	}
	if err := b.db.rlock(); err != nil {
		return err
	}
	defer b.db.mu.RUnlock()
	err := b.batch.Commit(pebble.Sync)
	if err != nil {
		return err
	}
	// Make sure batch cannot be used afterwards. Callers should still call Close(), for errors.
	return b.Close()
}

// Close implements Batch.
func (b *pebbleDBBatch) Close() error {
	// fmt.Println("pebbleDBBatch.Close")
	if b.batch != nil {
		err := b.batch.Close()
		if err != nil {
			return err
		}
		b.batch = nil
	}

	return nil
}

// GetByteSize implements Batch
func (b *pebbleDBBatch) GetByteSize() (int, error) {
	if b.batch == nil {
		return 0, errBatchClosed
	}
	return b.batch.Len(), nil
}

// pebbleDBIterator holds the DB read lock during each operation.
type pebbleDBIterator struct {
	db         *PebbleDB
	source     *pebble.Iterator // nil once closed
	err        error            // Error's result once closed
	start, end []byte
	isReverse  bool
	isInvalid  bool
}

var _ Iterator = (*pebbleDBIterator)(nil)

// newPebbleDBIterator must be called with db read-locked.
func newPebbleDBIterator(db *PebbleDB, source *pebble.Iterator, start, end []byte, isReverse bool) *pebbleDBIterator {
	if isReverse {
		if end == nil {
			source.Last()
		}
	} else {
		if start == nil {
			source.First()
		}
	}
	itr := &pebbleDBIterator{
		db:        db,
		source:    source,
		start:     start,
		end:       end,
		isReverse: isReverse,
		isInvalid: false,
	}
	db.itersMu.Lock()
	db.iters[itr] = struct{}{}
	db.itersMu.Unlock()
	return itr
}

// Domain implements Iterator.
func (itr *pebbleDBIterator) Domain() ([]byte, []byte) {
	// fmt.Println("pebbleDBIterator.Domain")
	return itr.start, itr.end
}

// Valid implements Iterator.
func (itr *pebbleDBIterator) Valid() bool {
	itr.db.mu.RLock()
	defer itr.db.mu.RUnlock()
	return itr.valid()
}

func (itr *pebbleDBIterator) valid() bool {
	// Once invalid, forever invalid.
	if itr.isInvalid {
		return false
	}

	// If closed, invalid.
	if itr.source == nil {
		itr.isInvalid = true

		return false
	}

	// If source has error, invalid.
	if err := itr.source.Error(); err != nil {
		itr.isInvalid = true

		return false
	}

	// If source is invalid, invalid.
	if !itr.source.Valid() {
		itr.isInvalid = true

		return false
	}

	// If key is end or past it, invalid.
	start := itr.start
	end := itr.end
	key := itr.source.Key()
	if itr.isReverse {
		if start != nil && bytes.Compare(key, start) < 0 {
			itr.isInvalid = true

			return false
		}
	} else {
		if end != nil && bytes.Compare(end, key) <= 0 {
			itr.isInvalid = true

			return false
		}
	}

	// It's valid.
	return true
}

// Key implements Iterator. It returns nil after the DB is closed.
func (itr *pebbleDBIterator) Key() []byte {
	itr.db.mu.RLock()
	defer itr.db.mu.RUnlock()
	if itr.source == nil {
		return nil
	}
	itr.assertIsValid()
	return cp(itr.source.Key())
}

// Value implements Iterator. It returns nil after the DB is closed.
func (itr *pebbleDBIterator) Value() []byte {
	itr.db.mu.RLock()
	defer itr.db.mu.RUnlock()
	if itr.source == nil {
		return nil
	}
	itr.assertIsValid()
	return cp(itr.source.Value())
}

// Next implements Iterator. It does nothing after the DB is closed.
func (itr *pebbleDBIterator) Next() {
	itr.db.mu.RLock()
	defer itr.db.mu.RUnlock()
	if itr.source == nil {
		return
	}
	itr.assertIsValid()
	if itr.isReverse {
		itr.source.Prev()
	} else {
		itr.source.Next()
	}
}

// Error implements Iterator.
func (itr *pebbleDBIterator) Error() error {
	itr.db.mu.RLock()
	defer itr.db.mu.RUnlock()
	if itr.source == nil {
		return itr.err
	}
	return itr.source.Error()
}

// Close implements Iterator.
func (itr *pebbleDBIterator) Close() error {
	itr.db.mu.RLock()
	defer itr.db.mu.RUnlock()
	if itr.source == nil {
		return nil
	}
	itr.db.itersMu.Lock()
	delete(itr.db.iters, itr)
	itr.db.itersMu.Unlock()
	itr.err = itr.source.Close()
	itr.source = nil
	return itr.err
}

func (itr *pebbleDBIterator) assertIsValid() {
	if !itr.valid() {
		panic("iterator is invalid")
	}
}

type fatalLogger struct {
	pebble.Logger
}

func (*fatalLogger) Fatalf(format string, args ...interface{}) {
	pebble.DefaultLogger.Fatalf(format, args...)
}

func (*fatalLogger) Infof(_ string, _ ...interface{}) {}
