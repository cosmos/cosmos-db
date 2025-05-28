all: lint test

### go tests
## By default this will only test memdb, goleveldb, and pebbledb, which do not require cgo
test:
	@echo "--> Running go test"
	@go test $(PACKAGES) -v

test-rocksdb:
	@echo "--> Running go test"
	@go test $(PACKAGES) -tags rocksdb -v

golangci_version=v2.1.6

#? lint-install: Install golangci-lint
lint-install:
	@echo "--> Installing golangci-lint $(golangci_version)"
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(golangci_version)
.PHONY: lint-install

lint: lint-install
	@echo "--> Running linter"
	@golangci-lint run

lint-fix: lint-install
	@echo "--> Running linter"
	@golangci-lint run --fix
.PHONY: lint lint-fix

