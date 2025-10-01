package main

import (
    "net/http"
    "os"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "go.uber.org/zap"
)

func getEnv(key, def string) string {
    v := os.Getenv(key)
    if v == "" {
        return def
    }
    return v
}

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    port := getEnv("PORT", "8080")
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(60 * time.Second))

    r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"status":"ok","service":"offer"}`))
    })
    r.Handle("/metrics", promhttp.Handler())

    addr := ":" + port
    logger.Info("starting offer-service", zap.String("addr", addr))
    if err := http.ListenAndServe(addr, r); err != nil {
        logger.Fatal("server failed", zap.Error(err))
    }
}

