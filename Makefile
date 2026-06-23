PKG ?= ./...

.DEFAULT_GOAL := help

.PHONY: help
help: ## Показать это сообщение
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Собрать пакет
	go build $(PKG)

.PHONY: vet
vet: ## Запустить go vet
	go vet $(PKG)

.PHONY: test
test: ## Запустить тесты с race-детектором и покрытием
	go test -race -coverprofile=coverage.out $(PKG)

.PHONY: cover
cover: test ## Открыть отчёт о покрытии в браузере
	go tool cover -html=coverage.out

.PHONY: lint
lint: ## Запустить golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Отформатировать код
	gofmt -s -w .

.PHONY: tidy
tidy: ## Привести в порядок go.mod/go.sum
	go mod tidy

.PHONY: check
check: vet lint test ## Полная проверка: vet + lint + test

.PHONY: tools
tools: ## Установить инструменты для разработки
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
