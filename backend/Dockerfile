FROM golang:1.17-alpine AS builder

RUN mkdir /build/
WORKDIR /build/
COPY go.* ./
RUN go mod download
COPY . ./
WORKDIR /build/cmd/web/
RUN go build -o webapp

FROM alpine
WORKDIR /app
COPY --from=builder /build/cmd/web/webapp /
ENTRYPOINT ./webapp

