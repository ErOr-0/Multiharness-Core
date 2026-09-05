.PHONY: check fmt test coverage race integration fuzz static security lint-workflows

# Never inherit opt-in live-agent execution into an ordinary development gate.
export MULTIHARNESS_SMOKE := 0
export MULTIHARNESS_SMOKE_FALLBACK := 0
export MULTIHARNESS_RUNTIME_CHECK := 0
export MULTIHARNESS_INSTALL_MODE := disabled

check: static test race fuzz

fmt:
	gofmt -w cmd internal

static:
	@test -z "$$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }
	go mod tidy -diff
	go mod verify
	go vet ./...
	go build ./...
	git diff --check

test:
	go test -count=1 -timeout 10m ./...

# Statement coverage of each package's own tests, including offline integration.
# Keep generated reports outside source and opt-in model calls disabled.
coverage:
	mkdir -p .coverage
	go test -count=1 -timeout 10m -coverprofile=.coverage/coverage.out ./...
	go tool cover -func=.coverage/coverage.out
	go tool cover -html=.coverage/coverage.out -o .coverage/index.html

race:
	go test -race -count=1 -timeout 10m ./...

integration:
	go test -count=1 -timeout 5m -v ./cmd/multiharness -run '^Test(Workflow|ProviderFailures)Integration$$'

fuzz:
	go test ./internal/adapter/agent/provider -run '^$$' -fuzz '^FuzzClassifyNeverLeaksRawErrors$$' -fuzztime=5s -parallel=2

# These two targets fetch pinned tools and may need network access. They do not
# alter go.mod, install global binaries, or receive provider credentials in CI.
security:
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -test ./...

lint-workflows:
	go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= .github/workflows/check.yml
