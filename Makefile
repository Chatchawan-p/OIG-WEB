.PHONY: build run test test-integration vet fmt swagger verify docker-up docker-down

build:
	go build -o bin/oig-api ./cmd/api
	go build -o bin/bootstrap-admin ./cmd/bootstrap-admin

bootstrap-admin:
	go run ./cmd/bootstrap-admin $(ARGS)

run:
	go run ./cmd/api

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

swagger:
	@if command -v swag >/dev/null 2>&1; then \
		swag init -g main.go -d cmd/api,internal/http/handler,internal/http/response,internal/dto -o docs --parseInternal; \
	elif [ -x "$$(go env GOPATH)/bin/swag" ]; then \
		"$$(go env GOPATH)/bin/swag" init -g main.go -d cmd/api,internal/http/handler,internal/http/response,internal/dto -o docs --parseInternal; \
	else \
		go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g main.go -d cmd/api,internal/http/handler,internal/http/response,internal/dto -o docs --parseInternal; \
	fi

test-integration:
	go test -tags=integration ./internal/repository/mongo

verify: test vet build

docker-up:
	docker compose up --build

docker-down:
	docker compose down
