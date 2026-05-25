FROM golang:1.26 AS base

WORKDIR /build/
COPY go.* ./
RUN go mod download
COPY . ./

FROM base AS cli

WORKDIR /build/cmd/cli/
RUN go build -o cli

ENTRYPOINT ["/build/cmd/cli/cli"]

FROM base AS builder

WORKDIR /build/cmd/web/
RUN CGO_ENABLED=0 GOOS=linux go build -o webapp

FROM alpine:3.20 AS web

RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /
COPY --from=builder /build/cmd/web/webapp /webapp
COPY --from=builder /build/layout/tmpl /layout/tmpl
USER app:app
ENTRYPOINT ["/webapp"]
