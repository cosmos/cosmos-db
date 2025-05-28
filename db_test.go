package db

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDBIteratorSingleKey(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)

			err := db.SetSync(bz("1"), bz("value_1"))
			require.NoError(t, err)
			itr, err := db.Iterator(nil, nil)
			require.NoError(t, err)

			checkValid(t, itr, true)
			checkNext(t, itr, false)
			checkValid(t, itr, false)
			checkNextPanics(t, itr)

			// Once invalid...
			checkInvalid(t, itr)
		})
	}
}

func TestDBIteratorTwoKeys(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)

			err := db.SetSync(bz("1"), bz("value_1"))
			require.NoError(t, err)

			err = db.SetSync(bz("2"), bz("value_1"))
			require.NoError(t, err)

			{ // Fail by calling Next too much
				itr, err := db.Iterator(nil, nil)
				require.NoError(t, err)
				checkValid(t, itr, true)

				checkNext(t, itr, true)
				checkValid(t, itr, true)

				checkNext(t, itr, false)
				checkValid(t, itr, false)

				checkNextPanics(t, itr)

				// Once invalid...
				checkInvalid(t, itr)
			}
		})
	}
}

func TestDBIteratorMany(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)

			keys := make([][]byte, 100)
			for i := 0; i < 100; i++ {
				keys[i] = []byte{byte(i)}
			}

			value := []byte{5}
			for _, k := range keys {
				err := db.Set(k, value)
				require.NoError(t, err)
			}

			itr, err := db.Iterator(nil, nil)
			require.NoError(t, err)

			defer itr.Close()
			for ; itr.Valid(); itr.Next() {
				key := itr.Key()
				value = itr.Value()
				value1, err := db.Get(key)
				require.NoError(t, err)
				require.Equal(t, value1, value)
			}
		})
	}
}

func TestDBIteratorEmpty(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)

			itr, err := db.Iterator(nil, nil)
			require.NoError(t, err)

			checkInvalid(t, itr)
		})
	}
}

func TestDBIteratorEmptyBeginAfter(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)

			itr, err := db.Iterator(bz("1"), nil)
			require.NoError(t, err)

			checkInvalid(t, itr)
		})
	}
}

func TestDBIteratorNonemptyBeginAfter(t *testing.T) {
	for backend := range backends {
		t.Run(fmt.Sprintf("Backend %s", backend), func(t *testing.T) {
			db, dir := newTempDB(t, backend)
			defer os.RemoveAll(dir)

			err := db.SetSync(bz("1"), bz("value_1"))
			require.NoError(t, err)
			itr, err := db.Iterator(bz("2"), nil)
			require.NoError(t, err)

			checkInvalid(t, itr)
		})
	}
}
