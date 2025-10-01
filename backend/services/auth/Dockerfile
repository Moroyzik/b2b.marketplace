FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/service ./

FROM gcr.io/distroless/base-debian12
WORKDIR /
ENV PORT=8080
COPY --from=builder /bin/service /service
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/service"]

