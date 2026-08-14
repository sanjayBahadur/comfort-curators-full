SHELL := /usr/bin/env bash

.PHONY: phase-1 phase-2 phase-3 phase-4 phase-5 phase-6 phase-7
.PHONY: security-scan secrets-scan dep-scan container-scan

phase-1:
	./scripts/run-phase 1

phase-2:
	./scripts/run-phase 2

phase-3:
	./scripts/run-phase 3

phase-4:
	./scripts/run-phase 4

phase-5:
	./scripts/run-phase 5

phase-6:
	./scripts/run-phase 6

phase-7:
	./scripts/run-phase 7

security-scan: secrets-scan dep-scan go-vet

secrets-scan:
	@echo "=== Secret Scan ==="
	@! grep -rE "(AKIA[0-9A-Z]{16}|sk-live-[a-zA-Z0-9]{20,}|xox[baprs]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]+|gh[pousr]_[a-zA-Z0-9]{36,}|-----BEGIN (RSA|EC|DSA|OPENSSH) PRIVATE KEY)" --include="*.go" --include="*.yaml" --include="*.json" --include="*.env" . || (echo "SECRETS FOUND - ABORTING" && exit 1)
	@echo "Secret scan passed"

dep-scan:
	@echo "=== Dependency Scan ==="
	go mod verify
	go mod tidy -diff
	@echo "Dependency scan passed"

go-vet:
	@echo "=== Static Analysis ==="
	go vet ./...
	@echo "Static analysis passed"

container-scan:
	@echo "=== Container Scan ==="
	@if [ -f Dockerfile ]; then \
		grep -q "^USER" Dockerfile && echo "USER directive found" || echo "WARNING: No USER directive in Dockerfile"; \
		grep -q "HEALTHCHECK" Dockerfile && echo "HEALTHCHECK found" || echo "WARNING: No HEALTHCHECK in Dockerfile"; \
	fi
	@echo "Container scan completed"

