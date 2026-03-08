.PHONY: build build-controller build-bridge build-wl-bridge build-kbeads build-coop build-beads3d test lint

# Build all components
build: build-controller build-bridge build-wl-bridge build-kbeads build-coop build-beads3d

# Go components (via go.work)
build-controller:
	cd gasboat/controller && go build ./cmd/controller/

build-bridge:
	cd gasboat/controller && go build ./cmd/slack-bridge/

build-wl-bridge:
	cd gasboat/controller && go build ./cmd/wl-bridge/

build-kbeads:
	cd kbeads && go build ./cmd/...

# Rust components
build-coop:
	cd coop && cargo build --workspace

# JS components
build-beads3d:
	cd beads3d && npm run build

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
