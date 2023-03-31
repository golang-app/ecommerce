FROM golang:1.20-alpine AS builder

WORKDIR /build/
COPY go.* ./
RUN go mod download
COPY . ./
WORKDIR /build/cmd/web/
RUN go build -o webapp

FROM alpine
WORKDIR /
COPY --from=builder /build/cmd/web/webapp /
ENTRYPOINT /webapp

