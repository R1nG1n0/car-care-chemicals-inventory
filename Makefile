BINARY  := garage
PKG     := ./cmd/garage
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build build-linux web dev run test vet clean

## build — сборка под текущую платформу (для локальной проверки)
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

## build-linux — статический бинарник под relay (amd64) со встроенным интерфейсом
build-linux: web
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64 $(PKG)

## web — интерфейс: зависимости строго по package-lock.json, проверка типов,
## линтер, тесты, сборка в web/dist (встраивается через go:embed)
web:
	cd web && npm ci && npm run lint && npm run test && npm run build

## dev — локальная разработка: сервер на тестовых данных (.dev/) и Vite
## с горячей перезагрузкой, /api проксируется на сервер. Открывать http://localhost:5173.
dev:
	@test -d web/node_modules || (cd web && npm ci)
	go build -o bin/$(BINARY)-dev $(PKG)
	@trap 'kill 0' INT TERM EXIT; \
		./bin/$(BINARY)-dev -config config/dev.json & \
		cd web && npx vite

## run — сервер на тестовых данных со встроенным интерфейсом (после make web)
run: build
	./bin/$(BINARY) -config config/dev.json

## test — все проверки: Go и интерфейс
test: vet
	go test ./...
	cd web && npm run lint && npm run test

vet:
	go vet ./...
	@test -z "$$(gofmt -l cmd internal web/embed.go)" || (gofmt -l cmd internal web/embed.go; echo "gofmt: файлы не отформатированы"; exit 1)

clean:
	rm -rf bin
	cd web && node scripts/clean-dist.mjs
