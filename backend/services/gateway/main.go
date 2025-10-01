package main

import (
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/go-chi/cors"
    "github.com/go-chi/httprate"
    "github.com/golang-jwt/jwt/v5"
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

func newReverseProxy(target string) (*httputil.ReverseProxy, error) {
    u, err := url.Parse(target)
    if err != nil {
        return nil, err
    }
    proxy := httputil.NewSingleHostReverseProxy(u)
    proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadGateway)
        _, _ = w.Write([]byte(`{"status":"error","error":{"code":"UPSTREAM_UNAVAILABLE","message":"upstream error"}}`))
    }
    return proxy, nil
}

func requireAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        authz := r.Header.Get("Authorization")
        if authz == "" || len(authz) < 8 || authz[:7] != "Bearer " {
            w.WriteHeader(http.StatusUnauthorized)
            _, _ = w.Write([]byte(`{"status":"error","error":{"code":"UNAUTHORIZED","message":"missing token"}}`))
            return
        }
        tokenStr := authz[7:]
        secret := getEnv("JWT_SECRET", "devsecret")
        token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
            if t.Method != jwt.SigningMethodHS256 {
                return nil, jwt.ErrTokenInvalidClaims
            }
            return []byte(secret), nil
        })
        if err != nil || !token.Valid {
            w.WriteHeader(http.StatusUnauthorized)
            _, _ = w.Write([]byte(`{"status":"error","error":{"code":"INVALID_TOKEN","message":"invalid token"}}`))
            return
        }
        next.ServeHTTP(w, r)
    })
}

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    port := getEnv("PORT", "8080")
    jwtSecret := getEnv("JWT_SECRET", "devsecret")
    _ = jwtSecret

    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(60 * time.Second))
    r.Use(httprate.LimitByIP(100, 1*time.Minute))
    r.Use(cors.Handler(cors.Options{
        AllowedOrigins:   []string{"*"},
        AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept"},
        ExposedHeaders:   []string{"Link"},
        AllowCredentials: true,
        MaxAge:           300,
    }))

    r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"status":"ok","service":"api-gateway"}`))
    })

    r.Handle("/metrics", promhttp.Handler())

    // Reverse proxies
    routes := map[string]string{
        "/api/v1/auth":      getEnv("AUTH_SERVICE_URL", "http://auth:8080"),
        "/api/v1/products":  getEnv("CATALOG_SERVICE_URL", "http://catalog:8080"),
        "/api/v1/offers":    getEnv("OFFER_SERVICE_URL", "http://offer:8080"),
        "/api/v1/search":    getEnv("SEARCH_SERVICE_URL", "http://search:8080"),
        "/api/v1/cart":      getEnv("CART_SERVICE_URL", "http://cart:8080"),
        "/api/v1/orders":    getEnv("ORDER_SERVICE_URL", "http://order:8080"),
        "/api/v1/suppliers": getEnv("SUPPLIER_SERVICE_URL", "http://supplier:8080"),
        "/api/v1/media":     getEnv("MEDIA_SERVICE_URL", "http://media:8080"),
        "/api/v1/notify":    getEnv("NOTIFICATION_SERVICE_URL", "http://notification:8080"),
        "/api/v1/users":     getEnv("USER_SERVICE_URL", "http://user:8080"),
        "/api/v1/admin":     getEnv("ADMIN_SERVICE_URL", "http://admin:8080"),
        "/api/v1/compare":   getEnv("COMPARE_SERVICE_URL", "http://compare:8080"),
    }

    for prefix, target := range routes {
        proxy, err := newReverseProxy(target)
        if err != nil {
            logger.Fatal("invalid proxy target", zap.String("prefix", prefix), zap.String("target", target), zap.Error(err))
        }
        handler := http.StripPrefix(prefix, proxy)
        // Require auth for cart and orders by default
        if prefix == "/api/v1/cart" || prefix == "/api/v1/orders" {
            r.Mount(prefix, requireAuth(handler))
        } else {
            r.Mount(prefix, handler)
        }
    }

    addr := ":" + port
    logger.Info("starting api-gateway", zap.String("addr", addr))
    if err := http.ListenAndServe(addr, r); err != nil {
        logger.Fatal("server failed", zap.Error(err))
    }
}

