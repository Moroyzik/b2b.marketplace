# syntax=docker/dockerfile:1

FROM golang:1.22 AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/catalog-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/catalog-service /app/catalog-service
COPY --from=builder /app/migrations /app/migrations
ENV PORT=8002
EXPOSE 8002
USER nonroot:nonroot
ENTRYPOINT ["/app/catalog-service"]

