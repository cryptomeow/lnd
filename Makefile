PKG := github.com/cryptomeow/lnd
ESCPKG := github.com\/lightningnetwork\/lnd
MOBILE_PKG := $(PKG)/mobile

BTCD_PKG := github.com/btcsuite/btcd
GOVERALLS_PKG := github.com/mattn/goveralls
LINT_PKG := github.com/golangci/golangci-lint/cmd/golangci-lint
GOACC_PKG := github.com/ory/go-acc
FALAFEL_PKG := github.com/lightninglabs/falafel
GOIMPORTS_PKG := golang.org/x/tools/cmd/goimports
GOFUZZ_BUILD_PKG := github.com/dvyukov/go-fuzz/go-fuzz-build
GOFUZZ_PKG := github.com/dvyukov/go-fuzz/go-fuzz

GO_BIN := ${GOPATH}/bin
BTCD_BIN := $(GO_BIN)/btcd
GOMOBILE_BIN := GO111MODULE=off $(GO_BIN)/gomobile
GOVERALLS_BIN := $(GO_BIN)/goveralls
LINT_BIN := $(GO_BIN)/golangci-lint
GOACC_BIN := $(GO_BIN)/go-acc
GOFUZZ_BUILD_BIN := $(GO_BIN)/go-fuzz-build
GOFUZZ_BIN := $(GO_BIN)/go-fuzz

BTCD_DIR :=${GOPATH}/src/$(BTCD_PKG)
MOBILE_BUILD_DIR :=${GOPATH}/src/$(MOBILE_PKG)/build
IOS_BUILD_DIR := $(MOBILE_BUILD_DIR)/ios
IOS_BUILD := $(IOS_BUILD_DIR)/Lndmobile.framework
ANDROID_BUILD_DIR := $(MOBILE_BUILD_DIR)/android
ANDROID_BUILD := $(ANDROID_BUILD_DIR)/Lndmobile.aar

COMMIT := $(shell git describe --abbrev=40 --dirty)
COMMIT_HASH := $(shell git rev-parse HEAD)

BTCD_COMMIT := $(shell cat go.mod | \
		grep $(BTCD_PKG) | \
		tail -n1 | \
		awk -F " " '{ print $$2 }' | \
		awk -F "/" '{ print $$1 }')

LINT_COMMIT := v1.18.0
GOACC_COMMIT := ddc355013f90fea78d83d3a6c71f1d37ac07ecd5
FALAFEL_COMMIT := v0.7.1
GOFUZZ_COMMIT := 21309f307f61

DEPGET := cd /tmp && GO111MODULE=on go get -v
GOBUILD := GO111MODULE=on go build -v
GOINSTALL := GO111MODULE=on go install -v
GOTEST := GO111MODULE=on go test 

GOVERSION := $(shell go version | awk '{print $$3}')
GOFILES_NOVENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

RM := rm -f
CP := cp
MAKE := make
XARGS := xargs -L 1

include make/testing_flags.mk
include make/release_flags.mk
include make/fuzz_flags.mk

DEV_TAGS := $(if ${tags},$(DEV_TAGS) ${tags},$(DEV_TAGS))

# We only return the part inside the double quote here to avoid escape issues
# when calling the external release script. The second parameter can be used to
# add additional ldflags if needed (currently only used for the release).
make_ldflags = $(2) -X $(PKG)/build.Commit=$(COMMIT) \
	-X $(PKG)/build.CommitHash=$(COMMIT_HASH) \
	-X $(PKG)/build.GoVersion=$(GOVERSION) \
	-X $(PKG)/build.RawTags=$(shell echo $(1) | sed -e 's/ /,/g')

LDFLAGS := -ldflags "$(call make_ldflags, ${tags}, -s -w)"
DEV_LDFLAGS := -ldflags "$(call make_ldflags, $(DEV_TAGS))"
ITEST_LDFLAGS := -ldflags "$(call make_ldflags, $(ITEST_TAGS))"

# For the release, we want to remove the symbol table and debug information (-s)
# and omit the DWARF symbol table (-w). Also we clear the build ID.
RELEASE_LDFLAGS := $(call make_ldflags, $(RELEASE_TAGS), -s -w -buildid=)

# Linting uses a lot of memory, so keep it under control by limiting the number
# of workers if requested.
ifneq ($(workers),)
LINT_WORKERS = --concurrency=$(workers)
endif

LINT = $(LINT_BIN) run -v $(LINT_WORKERS)

GREEN := "\\033[0;32m"
NC := "\\033[0m"
define print
	echo $(GREEN)$1$(NC)
endef

default: scratch

all: scratch check install

# ============
# DEPENDENCIES
# ============

$(GOVERALLS_BIN):
	@$(call print, "Fetching goveralls.")
	go get -u $(GOVERALLS_PKG)

$(LINT_BIN):
	@$(call print, "Fetching linter")
	$(DEPGET) $(LINT_PKG)@$(LINT_COMMIT)

$(GOACC_BIN):
	@$(call print, "Fetching go-acc")
	$(DEPGET) $(GOACC_PKG)@$(GOACC_COMMIT)

btcd:
	@$(call print, "Installing btcd.")
	$(DEPGET) $(BTCD_PKG)@$(BTCD_COMMIT)

falafel:
	@$(call print, "Installing falafel.")
	$(DEPGET) $(FALAFEL_PKG)@$(FALAFEL_COMMIT)

goimports:
	@$(call print, "Installing goimports.")
	$(DEPGET) $(GOIMPORTS_PKG)

$(GOFUZZ_BIN):
	@$(call print, "Fetching go-fuzz")
	$(DEPGET) $(GOFUZZ_PKG)@$(GOFUZZ_COMMIT)

$(GOFUZZ_BUILD_BIN):
	@$(call print, "Fetching go-fuzz-build")
	$(DEPGET) $(GOFUZZ_BUILD_PKG)@$(GOFUZZ_COMMIT)

# ============
# INSTALLATION
# ============

build:
	@$(call print, "Building debug lnd and lncli.")
	$(GOBUILD) -tags="$(DEV_TAGS)" -o lnd-debug $(DEV_LDFLAGS) $(PKG)/cmd/lnd
	$(GOBUILD) -tags="$(DEV_TAGS)" -o lncli-debug $(DEV_LDFLAGS) $(PKG)/cmd/lncli

build-itest:
	@$(call print, "Building itest lnd and lncli.")
	$(GOBUILD) -tags="$(ITEST_TAGS)" -o lnd-itest $(ITEST_LDFLAGS) $(PKG)/cmd/lnd
	$(GOBUILD) -tags="$(ITEST_TAGS)" -o lncli-itest $(ITEST_LDFLAGS) $(PKG)/cmd/lncli

build-itest-windows:
	@$(call print, "Building itest lnd and lncli.")
	$(GOBUILD) -tags="$(ITEST_TAGS)" -o lnd-itest.exe $(ITEST_LDFLAGS) $(PKG)/cmd/lnd
	$(GOBUILD) -tags="$(ITEST_TAGS)" -o lncli-itest.exe $(ITEST_LDFLAGS) $(PKG)/cmd/lncli

install:
	@$(call print, "Installing lnd and lncli.")
	$(GOINSTALL) -tags="${tags}" $(LDFLAGS) $(PKG)/cmd/lnd
	$(GOINSTALL) -tags="${tags}" $(LDFLAGS) $(PKG)/cmd/lncli

release:
	@$(call print, "Releasing lnd and lncli binaries.")
	$(VERSION_CHECK)
	./scripts/release.sh build-release "$(VERSION_TAG)" "$(BUILD_SYSTEM)" "$(RELEASE_TAGS)" "$(RELEASE_LDFLAGS)"

scratch: build


# =======
# TESTING
# =======

check: unit itest

itest-only:
	@$(call print, "Running integration tests with ${backend} backend.")
	$(ITEST)
	lntest/itest/log_check_errors.sh

itest: btcd build-itest itest-only

itest-parallel: btcd
	@$(call print, "Building lnd binary")
	CGO_ENABLED=0 $(GOBUILD) -tags="$(ITEST_TAGS)" -o lntest/itest/lnd-itest $(ITEST_LDFLAGS) $(PKG)/cmd/lnd

	@$(call print, "Building itest binary for $(backend) backend")
	CGO_ENABLED=0 $(GOTEST) -v ./lntest/itest -tags="$(DEV_TAGS) $(RPC_TAGS) rpctest $(backend)" -logoutput -goroutinedump -c -o lntest/itest/itest.test

	@$(call print, "Running tests")
	rm -rf lntest/itest/*.log lntest/itest/.logs-*
	echo -n "$$(seq 0 $$(expr $(NUM_ITEST_TRANCHES) - 1))" | xargs -P $(NUM_ITEST_TRANCHES) -n 1 -I {} scripts/itest_part.sh {} $(NUM_ITEST_TRANCHES) $(TEST_FLAGS)

itest-parallel-windows: btcd
	@$(call print, "Building lnd binary")
	CGO_ENABLED=0 $(GOBUILD) -tags="$(ITEST_TAGS)" -o lntest/itest/lnd-itest.exe $(ITEST_LDFLAGS) $(PKG)/cmd/lnd

	@$(call print, "Building itest binary for $(backend) backend")
	CGO_ENABLED=0 $(GOTEST) -v ./lntest/itest -tags="$(DEV_TAGS) $(RPC_TAGS) rpctest $(backend)" -logoutput -goroutinedump -c -o lntest/itest/itest.test.exe

	@$(call print, "Running tests")
	EXEC_SUFFIX=".exe" echo -n "$$(seq 0 $$(expr $(NUM_ITEST_TRANCHES) - 1))" | xargs -P $(NUM_ITEST_TRANCHES) -n 1 -I {} scripts/itest_part.sh {} $(NUM_ITEST_TRANCHES) $(TEST_FLAGS)

itest-windows: btcd build-itest-windows itest-only

unit: btcd
	@$(call print, "Running unit tests.")
	$(UNIT)

unit-cover: $(GOACC_BIN)
	@$(call print, "Running unit coverage tests.")
	$(GOACC_BIN) $(COVER_PKG) -- -tags="$(DEV_TAGS) $(LOG_TAGS)"


unit-race:
	@$(call print, "Running unit race tests.")
	env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(UNIT_RACE)

goveralls: $(GOVERALLS_BIN)
	@$(call print, "Sending coverage report.")
	$(GOVERALLS_BIN) -coverprofile=coverage.txt -service=travis-ci


travis-race: btcd unit-race

travis-cover: btcd unit-cover goveralls

# =============
# FLAKE HUNTING
# =============

flakehunter: build-itest
	@$(call print, "Flake hunting ${backend} integration tests.")
	while [ $$? -eq 0 ]; do $(ITEST); done

flake-unit:
	@$(call print, "Flake hunting unit tests.")
	while [ $$? -eq 0 ]; do GOTRACEBACK=all $(UNIT) -count=1; done

# =============
# FUZZING
# =============
fuzz-build: $(GOFUZZ_BUILD_BIN)
	@$(call print, "Creating fuzz harnesses for packages '$(FUZZPKG)'.")
	scripts/fuzz.sh build "$(FUZZPKG)"

fuzz-run: $(GOFUZZ_BIN)
	@$(call print, "Fuzzing packages '$(FUZZPKG)'.")
	scripts/fuzz.sh run "$(FUZZPKG)" "$(FUZZ_TEST_RUN_TIME)" "$(FUZZ_TEST_TIMEOUT)" "$(FUZZ_NUM_PROCESSES)" "$(FUZZ_BASE_WORKDIR)"

# =========
# UTILITIES
# =========

fmt:
	@$(call print, "Formatting source.")
	gofmt -l -w -s $(GOFILES_NOVENDOR)

lint: $(LINT_BIN)
	@$(call print, "Linting source.")
	$(LINT)

list:
	@$(call print, "Listing commands.")
	@$(MAKE) -qp | \
		awk -F':' '/^[a-zA-Z0-9][^$$#\/\t=]*:([^=]|$$)/ {split($$1,A,/ /);for(i in A)print A[i]}' | \
		grep -v Makefile | \
		sort

rpc:
	@$(call print, "Compiling protos.")
	cd ./lnrpc; ./gen_protos.sh

rpc-format:
	@$(call print, "Formatting protos.")
	cd ./lnrpc; find . -name "*.proto" | xargs clang-format --style=file -i

rpc-check: rpc
	@$(call print, "Verifying protos.")
	for rpc in $$(find lnrpc/ -name "*.proto" | $(XARGS) awk '/    rpc /{print $$2}'); do if ! grep -q $$rpc lnrpc/rest-annotations.yaml; then echo "RPC $$rpc not added to lnrpc/rest-annotations.yaml"; exit 1; fi; done
	if test -n "$$(git describe --dirty | grep dirty)"; then echo "Protos not properly formatted or not compiled with v3.4.0"; git status; git diff; exit 1; fi

sample-conf-check:
	@$(call print, "Making sure every flag has an example in the sample-lnd.conf file")
	for flag in $$(GO_FLAGS_COMPLETION=1 go run -tags="$(RELEASE_TAGS)" $(PKG)/cmd/lnd -- | grep -v help | cut -c3-); do if ! grep -q $$flag sample-lnd.conf; then echo "Command line flag --$$flag not added to sample-lnd.conf"; exit 1; fi; done

mobile-rpc: falafel goimports
	@$(call print, "Creating mobile RPC from protos.")
	cd ./mobile; ./gen_bindings.sh $(FALAFEL_COMMIT)

vendor:
	@$(call print, "Re-creating vendor directory.")
	rm -r vendor/; GO111MODULE=on go mod vendor

ios: vendor mobile-rpc
	@$(call print, "Building iOS framework ($(IOS_BUILD)).")
	mkdir -p $(IOS_BUILD_DIR)
	$(GOMOBILE_BIN) bind -target=ios -tags="mobile $(DEV_TAGS) autopilotrpc" $(LDFLAGS) -v -o $(IOS_BUILD) $(MOBILE_PKG)

android: vendor mobile-rpc
	@$(call print, "Building Android library ($(ANDROID_BUILD)).")
	mkdir -p $(ANDROID_BUILD_DIR)
	$(GOMOBILE_BIN) bind -target=android -tags="mobile $(DEV_TAGS) autopilotrpc" $(LDFLAGS) -v -o $(ANDROID_BUILD) $(MOBILE_PKG)

mobile: ios android

clean:
	@$(call print, "Cleaning source.$(NC)")
	$(RM) ./lnd-debug ./lncli-debug
	$(RM) ./lnd-itest ./lncli-itest
	$(RM) -r ./vendor .vendor-new


.PHONY: all \
	btcd \
	default \
	build \
	install \
	scratch \
	check \
	itest-only \
	itest \
	unit \
	unit-cover \
	unit-race \
	falafel \
	goveralls \
	travis-race \
	travis-cover \
	travis-itest \
	flakehunter \
	flake-unit \
	fmt \
	lint \
	list \
	rpc \
	rpc-format \
	rpc-check \
	mobile-rpc \
	vendor \
	ios \
	android \
	mobile \
	clean
