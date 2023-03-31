
tests:
	@go test ./...

integration:
	@go test -tags=integration ./... -count 1

swagger:
	@swag init -g ./cmd/web/main.go
