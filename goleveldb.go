package db

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cast"
	"github.com/syndtr/goleveldb/leveldb"
	leveldberrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func init() {
	dbCreator := func(name, dir string, opts Options) (DB, error) {
		return NewGoLevelDB(name, dir, opts)
	}
	registerDBCreator(GoLevelDBBackend, dbCreator, false)
}

type GoLevelDB struct {
	db *leveldb.DB
}

var _ DB = (*GoLevelDB)(nil)

func NewGoLevelDB(name, dir string, opts Options) (*GoLevelDB, error) {
	defaultOpts := &opt.Options{
		Filter: filter.NewBloomFilter(10), // by default, goleveldb doesn't use a bloom filter.
	}
	if opts != nil {
		files := cast.ToInt(opts.Get("maxopenfiles"))
		if files > 0 {
			defaultOpts.OpenFilesCacheCapacity = files
		}
	}

	return NewGoLevelDBWithOpts(name, dir, defaultOpts)
}

func NewGoLevelDBWithOpts(name, dir string, o *opt.Options) (*GoLevelDB, error) {
	dbPath := filepath.Join(dir, name+DBFileSuffix)
	db, err := leveldb.OpenFile(dbPath, o)
	if err != nil {
		return nil, err
	}
	database := &GoLevelDB{
		db: db,
	}
	return database, nil
}

// Get implements DB.
func (db *GoLevelDB) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errKeyEmpty
	}
	res, err := db.db.Get(key, nil)
	if err != nil {
		if errors.Is(err, leveldberrors.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}

// Has implements DB.
func (db *GoLevelDB) Has(key []byte) (bool, error) {
	bytes, err := db.Get(key)
	if err != nil {
		return false, err
	}
	return bytes != nil, nil
}

// Set implements DB.
func (db *GoLevelDB) Set(key, value []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	if value == nil {
		return errValueNil
	}
	if err := db.db.Put(key, value, nil); err != nil {
		return err
	}
	return nil
}

// SetSync implements DB.
func (db *GoLevelDB) SetSync(key, value []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	if value == nil {
		return errValueNil
	}
	if err := db.db.Put(key, value, &opt.WriteOptions{Sync: true}); err != nil {
		return err
	}
	return nil
}

// Delete implements DB.
func (db *GoLevelDB) Delete(key []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	if err := db.db.Delete(key, nil); err != nil {
		return err
	}
	return nil
}

// DeleteSync implements DB.
func (db *GoLevelDB) DeleteSync(key []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	err := db.db.Delete(key, &opt.WriteOptions{Sync: true})
	if err != nil {
		return err
	}
	return nil
}

func (db *GoLevelDB) DB() *leveldb.DB {
	return db.db
}

// Close implements DB.
func (db *GoLevelDB) Close() error {
	if err := db.db.Close(); err != nil {
		return err
	}
	return nil
}

// Print implements DB.
func (db *GoLevelDB) Print() error {
	str, err := db.db.GetProperty("leveldb.stats")
	if err != nil {
		return err
	}
	fmt.Printf("%v\n", str)

	itr := db.db.NewIterator(nil, nil)
	for itr.Next() {
		key := itr.Key()
		value := itr.Value()
		fmt.Printf("[%X]:\t[%X]\n", key, value)
	}
	return nil
}

// Stats implements DB.
func (db *GoLevelDB) Stats() map[string]string {
	keys := []string{
		"leveldb.num-files-at-level{n}",
		"leveldb.stats",
		"leveldb.sstables",
		"leveldb.blockpool",
		"leveldb.cachedblock",
		"leveldb.openedtables",
		"leveldb.alivesnaps",
		"leveldb.aliveiters",
	}

	stats := make(map[string]string)
	for _, key := range keys {
		str, err := db.db.GetProperty(key)
		if err == nil {
			stats[key] = str
		}
	}
	return stats
}

func (db *GoLevelDB) ForceCompact(start, limit []byte) error {
	return db.db.CompactRange(util.Range{Start: start, Limit: limit})
}

// NewBatch implements DB.
func (db *GoLevelDB) NewBatch() Batch {
	return newGoLevelDBBatch(db)
}

// NewBatchWithSize implements DB.
func (db *GoLevelDB) NewBatchWithSize(size int) Batch {
	return newGoLevelDBBatchWithSize(db, size)
}

// Iterator implements DB.
func (db *GoLevelDB) Iterator(start, end []byte) (Iterator, error) {
	if (start != nil && len(start) == 0) || (end != nil && len(end) == 0) {
		return nil, errKeyEmpty
	}
	itr := db.db.NewIterator(&util.Range{Start: start, Limit: end}, nil)
	return newGoLevelDBIterator(itr, start, end, false), nil
}

// ReverseIterator implements DB.
func (db *GoLevelDB) ReverseIterator(start, end []byte) (Iterator, error) {
	if (start != nil && len(start) == 0) || (end != nil && len(end) == 0) {
		return nil, errKeyEmpty
	}
	itr := db.db.NewIterator(&util.Range{Start: start, Limit: end}, nil)
	return newGoLevelDBIterator(itr, start, end, true), nil
}
