.PHONY: check test race acceptance fuzz static security lint-workflows

# Never inherit opt-in live-agent execution into an ordinary development gate.
export MULTIHARNESS_SMOKE := 0
export MULTIHARNESS_SMOKE_FALLBACK := 0
export MULTIHARNESS_RUNTIME_CHECK := 0

check: static test race fuzz

static:
	@test -z "$$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }
	go mod tidy -diff
	go mod verify
	go vet ./...
	go build ./...
	git diff --check

test:
	go test -count=1 -timeout 10m ./...

race:
	go test -race -count=1 -timeout 10m ./...

acceptance:
	go test -count=1 -timeout 5m -v ./cmd/multiharness -run '^TestAcceptanceFeatures$$'

fuzz:
	go test ./internal/adapter/agent/provider -run '^$$' -fuzz '^FuzzClassifyNeverLeaksRawErrors$$' -fuzztime=5s -parallel=2

# These two targets fetch pinned tools and may need network access. They do not
# alter go.mod, install global binaries, or receive provider credentials in CI.
security:
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -test ./...

lint-workflows:
	go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= .github/workflows/check.yml
