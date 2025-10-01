# syntax=docker/dockerfile:1

FROM golang:1.22 AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/auth-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/auth-service /app/auth-service
COPY --from=builder /app/migrations /app/migrations
ENV PORT=8001
EXPOSE 8001
USER nonroot:nonroot
ENTRYPOINT ["/app/auth-service"]

