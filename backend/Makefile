
tests:
	@go test ./...

integration:
	@go test -tags=integration ./... -count 1

openapi-docs:
	@swag init -g ./cmd/web/main.go
