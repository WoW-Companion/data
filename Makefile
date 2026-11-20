export CGO_ENABLED = 0

BUILD ?= 1.60.1.71037

.PHONY: verify fmt fetch dataset spike

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

spike:
	go test ./spikes/parquet_layout -run '^$$' -bench . -benchtime=1x
