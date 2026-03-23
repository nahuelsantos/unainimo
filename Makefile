# unainimo — local orchestration (Docker Compose)
#
# All compose commands use a fixed project name (see COMPOSE_PROJECT) and the
# directory of this Makefile, so stop/clean reliably release host ports for the
# next `make start`. `start` runs `down` first to drop any leftover containers
# from this project.

COMPOSE ?= $(shell \
	if docker compose version >/dev/null 2>&1; then \
		printf '%s' 'docker compose'; \
	elif command -v docker-compose >/dev/null 2>&1; then \
		printf '%s' 'docker-compose'; \
	else \
		printf '%s' 'docker compose'; \
	fi)

# Directory containing this Makefile (repo root when Makefile lives there).
ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
COMPOSE_FILE := $(ROOT)docker-compose.yml
# Fixed stack name so stop/clean/start always refer to the same containers/volumes.
COMPOSE_PROJECT ?= unainimo

DEV_PORT ?= 8080
# Defaults must match docker-compose (APP_HOST_PORT / REDIS_HOST_PORT).
APP_PORT ?= 8888
REDIS_PORT ?= 6380

.PHONY: help start stop clean restart logs ps build dev ports teardown

# Single shell prefix: repo root + explicit compose file + fixed project name.
COMPOSE_EXEC = cd $(ROOT) && $(COMPOSE) -f $(COMPOSE_FILE) -p $(COMPOSE_PROJECT)

help:
	@echo "Targets:"
	@echo "  make start   - tear down this project's stack, then build & run app + Redis (detached)"
	@echo "  make stop    - docker compose down (containers + network removed; volumes kept)"
	@echo "  make clean   - down + delete volumes (Redis data) + remove orphans"
	@echo "  make restart - stop then start"
	@echo "  make logs    - follow app + redis logs"
	@echo "  make ps      - compose status"
	@echo "  make build   - build images only"
	@echo "  make ports   - show listeners on app/redis host ports (see APP_PORT / REDIS_PORT)"
	@echo "  make dev     - go run locally (no Docker)"
	@echo ""
	@echo "Compose: $(COMPOSE)  |  project: $(COMPOSE_PROJECT)  |  file: $(COMPOSE_FILE)"
	@echo "Docker UI: http://localhost:$(APP_PORT) (set APP_HOST_PORT in .env to change)"

start: teardown
	$(COMPOSE_EXEC) up -d --build --remove-orphans

# Full teardown for this Compose project (do not hide errors — was masking failed downs).
teardown:
	-$(COMPOSE_EXEC) down --remove-orphans --timeout 30
	@sh -c 'ids=$$(docker ps -aq -f label=com.docker.compose.project=$(COMPOSE_PROJECT) 2>/dev/null); \
		if [ -n "$$ids" ]; then echo "Removing leftover containers: $$ids"; docker rm -f $$ids; fi'

stop:
	$(COMPOSE_EXEC) down --remove-orphans --timeout 30
	@sh -c 'ids=$$(docker ps -aq -f label=com.docker.compose.project=$(COMPOSE_PROJECT) 2>/dev/null); \
		if [ -n "$$ids" ]; then echo "Removing leftover containers: $$ids"; docker rm -f $$ids; fi'

clean:
	$(COMPOSE_EXEC) down -v --remove-orphans --timeout 30
	@sh -c 'ids=$$(docker ps -aq -f label=com.docker.compose.project=$(COMPOSE_PROJECT) 2>/dev/null); \
		if [ -n "$$ids" ]; then echo "Removing leftover containers: $$ids"; docker rm -f $$ids; fi'

restart: stop start

logs:
	$(COMPOSE_EXEC) logs -f unanimo-app redis

ps:
	$(COMPOSE_EXEC) ps

build:
	$(COMPOSE_EXEC) build

ports:
	@echo "Listeners on APP_HOST_PORT default ($(APP_PORT)) and REDIS_HOST_PORT default ($(REDIS_PORT)):"
	@lsof -nP -iTCP:$(APP_PORT) -sTCP:LISTEN 2>/dev/null || echo "  (none on :$(APP_PORT))"
	@lsof -nP -iTCP:$(REDIS_PORT) -sTCP:LISTEN 2>/dev/null || echo "  (none on :$(REDIS_PORT))"

dev:
	PORT=$(DEV_PORT) TEMPLATES_DIR=$(ROOT)web/templates STATIC_DIR=$(ROOT)web/static \
		go run $(ROOT)cmd/server
