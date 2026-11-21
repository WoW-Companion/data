export CGO_ENABLED = 0

BUILD ?= 1.60.1.71037

.PHONY: verify fmt fetch dataset bench spike release-dryrun

verify:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...

fmt:
	gofmt -w .

fetch:
	go run ./cmd/wowc-data fetch -build $(BUILD)

dataset:
	go run ./cmd/wowc-data build -build $(BUILD)
	go run ./cmd/wowc-data verify-manifest dist/$(BUILD)

bench:
	go run ./cmd/wowc-data bench dist/$(BUILD)

spike:
	go test ./spikes/parquet_layout -run '^$$' -bench . -benchtime=1x

# release-dryrun runs the dataset workflow's job on this machine, in its order, without
# publishing: release-check, fetch, build, verify-manifest, pack, release notes as the
# first release and as a diff against the dataset itself. Needs the network and zstd.
REPO ?= WoW-Companion/data
DRY = dist/release-dryrun

release-dryrun:
	@echo "==> go version (go.mod wants $$(awk '/^go /{print $$2}' go.mod))"; go version
	@echo "==> release-check"; GITHUB_REPOSITORY=$(REPO) go run ./cmd/wowc-data release-check
	@echo "==> fetch"; go run ./cmd/wowc-data fetch -build $(BUILD)
	@echo "==> build"; go run ./cmd/wowc-data build -build $(BUILD)
	@echo "==> verify-manifest"; go run ./cmd/wowc-data verify-manifest dist/$(BUILD)
	@echo "==> pack, previous release (none) and notes"; rm -rf $(DRY); BUILD=$(BUILD) DRYRUN=1 PREV_TAG= OUT=$(DRY) scripts/pack-and-publish.sh
	@echo "==> notes against a previous release (the dataset itself)"; mkdir $(DRY)/prev; cp $(DRY)/manifest-$(BUILD).json $(DRY)/prev/manifest.json; cp $(DRY)/stats-$(BUILD).json $(DRY)/prev/stats.json
	@go run ./cmd/wowc-data release-notes -prev-tag dataset-$(BUILD) -prev-dir $(DRY)/prev dist/$(BUILD) | head -20
	@echo "==> release-dryrun passed; nothing was published"
