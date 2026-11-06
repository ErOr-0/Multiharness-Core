.PHONY: check fmt static test race lint security lint-workflows install build
.DEFAULT_GOAL := check

# Only ./cmd/... is built and checked, so ignored local folders such as dist/
# never break the gate.
PACKAGES := ./cmd/...

check: static test race

fmt:
	gofmt -w cmd

static:
	@test -z "$$(gofmt -l cmd)" || { gofmt -l cmd; exit 1; }
	go mod tidy -diff
	go mod verify
	go vet $(PACKAGES)
	GOOS=linux go vet $(PACKAGES)
	GOOS=darwin go vet $(PACKAGES)
	GOOS=windows go vet $(PACKAGES)
	go build $(PACKAGES)
	git diff --check

test:
	go test -count=1 -timeout 5m $(PACKAGES)

race:
	go test -race -count=1 -timeout 5m $(PACKAGES)

# Puts harness-bridge in $(go env GOPATH)/bin, which must be on PATH for agents.
install:
	go install ./cmd/harness-bridge

build:
	mkdir -p dist
	go build -o dist/ ./cmd/harness-bridge

# These targets fetch pinned tools and may need network access.
lint:
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 $(PACKAGES)

security:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -test $(PACKAGES)

lint-workflows:
	go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= .github/workflows/check.yml .github/workflows/release.yml
