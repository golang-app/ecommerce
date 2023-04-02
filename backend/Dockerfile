FROM golang:1.18-alpine as base

WORKDIR /build/
COPY go.* ./
RUN go mod download
COPY . ./

FROM base as cli

WORKDIR /build/cmd/cli/
RUN go build -o cli

ENTRYPOINT ["/build/cmd/cli/cli"]

FROM base AS builder

WORKDIR /build/cmd/web/
RUN go build -o webapp

FROM alpine as web

WORKDIR /
COPY --from=builder /build/cmd/web/webapp /
ENTRYPOINT /webapp
