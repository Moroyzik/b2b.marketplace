package main

import (
    "encoding/json"
    "net/http"
    "os"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/google/uuid"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "go.uber.org/zap"
)

type Product struct {
    ID          uuid.UUID              `json:"id"`
    SKU         string                 `json:"sku"`
    Title       string                 `json:"title"`
    Description string                 `json:"description"`
    CategoryID  *uuid.UUID             `json:"category_id"`
    Attributes  map[string]interface{} `json:"attributes"`
    Thumbnail   string                 `json:"thumbnail_url"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
    IsActive    bool                   `json:"is_active"`
}

var products = map[string]Product{}

func getEnv(key, def string) string {
    v := os.Getenv(key)
    if v == "" {
        return def
    }
    return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func listProducts(w http.ResponseWriter, r *http.Request) {
    list := make([]Product, 0, len(products))
    for _, p := range products {
        list = append(list, p)
    }
    writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "data": map[string]any{"items": list, "meta": map[string]int{"total": len(list)}}})
}

func getProduct(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    p, ok := products[id]
    if !ok {
        writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "error": map[string]string{"code": "NOT_FOUND", "message": "product not found"}})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "data": p})
}

func createProduct(w http.ResponseWriter, r *http.Request) {
    var p Product
    if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": map[string]string{"code": "BAD_REQUEST", "message": "invalid body"}})
        return
    }
    p.ID = uuid.New()
    p.CreatedAt = time.Now()
    p.UpdatedAt = time.Now()
    p.IsActive = true
    products[p.ID.String()] = p
    writeJSON(w, http.StatusCreated, map[string]any{"status": "ok", "data": p})
}

func updateProduct(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    existing, ok := products[id]
    if !ok {
        writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "error": map[string]string{"code": "NOT_FOUND", "message": "product not found"}})
        return
    }
    var p Product
    if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": map[string]string{"code": "BAD_REQUEST", "message": "invalid body"}})
        return
    }
    p.ID = existing.ID
    p.CreatedAt = existing.CreatedAt
    p.UpdatedAt = time.Now()
    products[id] = p
    writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "data": p})
}

func deleteProduct(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    if _, ok := products[id]; !ok {
        writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "error": map[string]string{"code": "NOT_FOUND", "message": "product not found"}})
        return
    }
    delete(products, id)
    writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "catalog"})
    })
    r.Handle("/metrics", promhttp.Handler())

    r.Route("/api/v1/products", func(sr chi.Router) {
        sr.Get("/", listProducts)
        sr.Post("/", createProduct)
        sr.Route("/{id}", func(rr chi.Router) {
            rr.Get("/", getProduct)
            rr.Put("/", updateProduct)
            rr.Delete("/", deleteProduct)
        })
    })

    addr := ":" + port
    logger.Info("starting catalog-service", zap.String("addr", addr))
    if err := http.ListenAndServe(addr, r); err != nil {
        logger.Fatal("server failed", zap.Error(err))
    }
}

