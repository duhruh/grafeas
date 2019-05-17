FROM golang:1.12.5 as base
COPY . /app
WORKDIR /app

FROM base as dev
CMD go run samples/server/go-server/api/server/cmd/server/main.go

FROM base as builder
RUN CGO_ENABLED=0 go build -o grafeas-server samples/server/go-server/api/server/cmd/server/main.go

FROM alpine:latest
WORKDIR /
COPY --from=builder /app/grafeas-server /grafeas-server
EXPOSE 8080
ENTRYPOINT ["/grafeas-server"]
