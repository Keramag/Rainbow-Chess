# Makefile — single entry point for the test suites so CI and local match.
# `make test` runs exactly what .github/workflows/deploy.yml runs.

.PHONY: test test-backend test-frontend

# Run every test in the project (backend Go + frontend Node).
test: test-backend test-frontend

# Engine + hub/server tests.
test-backend:
	cd backend && go test ./...

# Pure ES-module frontend logic tests (Node's built-in runner, no deps).
test-frontend:
	node --test
