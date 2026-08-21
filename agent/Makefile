MODULE=risklens/agent
BINARY=RiskLensAgent.exe
VERSION?=0.1.0
COMMIT?=unknown
BUILDTIME?=unknown

LDFLAGS=-X $(MODULE)/internal/version.Version=$(VERSION) -X $(MODULE)/internal/version.Commit=$(COMMIT) -X $(MODULE)/internal/version.BuildTime=$(BUILDTIME)

.PHONY: build test vet

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/risklens-agent

test:
	go test ./...

vet:
	go vet ./...
