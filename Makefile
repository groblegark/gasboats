.PHONY: build build-gasboat build-kbeads build-coop build-beads3d test lint clean

# Build all components
build: build-gasboat build-kbeads build-coop build-beads3d

# Go components (via go.work)
build-gasboat:
	cd gasboat/controller && CGO_ENABLED=0 go build -o bin/controller ./cmd/controller/
	cd gasboat/controller && CGO_ENABLED=0 go build -o bin/slack-bridge ./cmd/slack-bridge/
	cd gasboat/controller && CGO_ENABLED=0 go build -o bin/gb ./cmd/gb/
	cd gasboat/controller && CGO_ENABLED=0 go build -o bin/jira-bridge ./cmd/jira-bridge/
	cd gasboat/controller && CGO_ENABLED=0 go build -o bin/gitlab-bridge ./cmd/gitlab-bridge/
	cd gasboat/controller && CGO_ENABLED=0 go build -o bin/advice-viewer ./cmd/advice-viewer/

build-kbeads:
	cd kbeads && CGO_ENABLED=0 go build -o bin/kd ./cmd/kd/

# Rust components
build-coop:
	cd coop && cargo build --workspace --release

# JS components
build-beads3d:
	cd beads3d && npm ci && npm run build

# Test all
test: test-go test-rust test-js

test-go:
	cd gasboat/controller && go test ./...
	cd kbeads && go test ./...

test-rust:
	cd coop && cargo test --workspace

test-js:
	cd beads3d && npm test

# Lint
lint:
	cd gasboat/controller && go vet ./...
	cd kbeads && go vet ./...
	cd coop && cargo clippy --workspace

# Clean
clean:
	rm -rf gasboat/controller/bin/
	rm -rf kbeads/bin/
	cd coop && cargo clean
	rm -rf beads3d/dist/
