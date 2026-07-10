APP_NAME := it-tools-portal
BIN_DIR := bin
WEB_DIR := apps/web
EMBED_DIR := internal/handlers/web/dist

.PHONY: build web-build sync-web run test docker-build docker-up docker-down clean

build: web-build sync-web
	go build -o $(BIN_DIR)/$(APP_NAME) ./cmd/server

web-build:
	npm --prefix $(WEB_DIR) install
	npm --prefix $(WEB_DIR) run build

sync-web:
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	cp -R $(WEB_DIR)/dist/. $(EMBED_DIR)/

run: build
	PORT=$${PORT:-8080} ./$(BIN_DIR)/$(APP_NAME)

test: web-build sync-web
	go test ./...

docker-build:
	docker build -t $(APP_NAME):local .

docker-up:
	docker compose up --build

docker-down:
	docker compose down --remove-orphans

clean:
	rm -rf $(BIN_DIR) $(WEB_DIR)/dist $(EMBED_DIR)/assets
