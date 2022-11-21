//go:build cleveldb
// +build cleveldb

package db

import "github.com/jmhodges/levigo"

// cLevelDBBatch is a LevelDB batch.
type cLevelDBBatch struct {
	db    *CLevelDB
	batch *levigo.WriteBatch
	size  uint64
}

func newCLevelDBBatch(db *CLevelDB) *cLevelDBBatch {
	return &cLevelDBBatch{
		db:    db,
		batch: levigo.NewWriteBatch(),
		size:  0,
	}
}

// Set implements Batch.
func (b *cLevelDBBatch) Set(key, value []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	if value == nil {
		return errValueNil
	}
	if b.batch == nil {
		return errBatchClosed
	}
	b.size += uint64(len(key) + len(value))
	b.batch.Put(key, value)
	return nil
}

// Delete implements Batch.
func (b *cLevelDBBatch) Delete(key []byte) error {
	if len(key) == 0 {
		return errKeyEmpty
	}
	if b.batch == nil {
		return errBatchClosed
	}
	b.size += uint64(len(key))
	b.batch.Delete(key)
	return nil
}

// Write implements Batch.
func (b *cLevelDBBatch) Write() error {
	if b.batch == nil {
		return errBatchClosed
	}
	err := b.db.db.Write(b.db.wo, b.batch)
	if err != nil {
		return err
	}
	// Make sure batch cannot be used afterwards. Callers should still call Close(), for errors.
	return b.Close()
}

// WriteSync implements Batch.
func (b *cLevelDBBatch) WriteSync() error {
	if b.batch == nil {
		return errBatchClosed
	}
	err := b.db.db.Write(b.db.woSync, b.batch)
	if err != nil {
		return err
	}
	// Make sure batch cannot be used afterwards. Callers should still call Close(), for errors.
	b.Close()
	return nil
}

// Close implements Batch.
func (b *cLevelDBBatch) Close() error {
	if b.batch != nil {
		b.batch.Close()
		b.batch = nil
		b.size = 0
	}
	return nil
}

// GetByteSize implements Batch
func (b *cLevelDBBatch) GetByteSize() (uint64, error) {
	if b.batch == nil {
		return 0, errBatchClosed
	}
	return b.size, nil
}
