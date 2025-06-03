# Changelog

## [v1.1.3] - 2025-06-03

* Revert commit `38785e92904d435a97e0d1b171089278bddf6760` - "Make `Iterator` and `Batch` interfaces more flexible by a type alias"

## [v1.1.2] - 2025-05-28 (RETRACTED)

* Make `Iterator` and `Batch` interfaces more flexible by a type alias
* Update deps to the latest versions
* Update linter for general code cleanup

## [v1.1.1] - 2024-12-19

* [#120](https://github.com/cosmos/cosmos-db/pull/120) Skip unwanted logs from PebbleDB

## [v1.1.0] - 2024-11-22

* Allow full control in rocksdb opening
* Remove build tag for PebbleDB

## [v1.0.2] - 2024-02-26

* Downgrade Go version in `go.mod` to 1.19

## [v1.0.1] - 2024-02-25

## [v1.0.0] - 2023-05-25

> Note this repository was forked from [github.com/tendermint/tm-db](https://github.com/tendermint/tm-db). Minor modifications were made after the fork to better support the Cosmos SDK. Notably, this repo removes badger, boltdb and cleveldb.

* added bloom filter:  <https://github.com/cosmos/cosmos-db/pull/42/files>
* Removed Badger & Boltdb
* Add `NewBatchWithSize` to `DB` interface: <https://github.com/cosmos/cosmos-db/pull/64>
* Add `NewRocksDBWithRaw` to support different rocksdb open mode (read-only, secondary-standby).
