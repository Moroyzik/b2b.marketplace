package main

import (
    "context"
    "crypto/rsa"
    "crypto/x509"
    "database/sql"
    "encoding/json"
    "encoding/pem"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    jwt "github.com/golang-jwt/jwt/v5"
    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
)

type AppConfig struct {
    Port        string
    DatabaseURL string
    JWTPublicPEM string
}

type Product struct {
    ID          string                 `db:"id" json:"id"`
    SKU         string                 `db:"sku" json:"sku"`
    Title       string                 `db:"title" json:"title"`
    Description string                 `db:"description" json:"description"`
    CategoryID  *string                `db:"category_id" json:"category_id,omitempty"`
    Attributes  map[string]interface{} `db:"attributes" json:"attributes"`
    Thumbnail   *string                `db:"thumbnail_url" json:"thumbnail_url,omitempty"`
    CreatedAt   time.Time              `db:"created_at" json:"created_at"`
    UpdatedAt   time.Time              `db:"updated_at" json:"updated_at"`
    IsActive    bool                   `db:"is_active" json:"is_active"`
}

var (
    db *sqlx.DB
    verifier *rsa.PublicKey
)

func main() {
    cfg := AppConfig{
        Port:        getEnv("PORT", "8002"),
        DatabaseURL: getEnv("DATABASE_URL", "postgres://dev:dev@localhost:5432/marketplace?sslmode=disable"),
        JWTPublicPEM: os.Getenv("JWT_PUBLIC_KEY"),
    }

    var err error
    db, err = sqlx.Connect("postgres", cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("db connect: %v", err)
    }
    if err := ensureMigrations(db.DB); err != nil {
        log.Fatalf("migrations: %v", err)
    }

    // Parse RSA public key if provided for JWT verification
    if cfg.JWTPublicPEM != "" {
        key, err := parseRSAPublicKeyFromPEM([]byte(cfg.JWTPublicPEM))
        if err != nil {
            log.Fatalf("invalid JWT_PUBLIC_KEY: %v", err)
        }
        verifier = key
    }

    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    r.Get("/api/v1/products", listProducts)
    r.Group(func(admin chi.Router) {
        admin.Use(jwtAuthMiddleware())
        admin.Use(requireAdmin())
        admin.Post("/api/v1/products", createProduct)
    })

    addr := ":" + cfg.Port
    log.Printf("catalog-service listening on %s", addr)
    if err := http.ListenAndServe(addr, r); err != nil {
        log.Fatalf("server error: %v", err)
    }
}

func ensureMigrations(sqldb *sql.DB) error {
    _, err := sqldb.Exec(`
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE IF NOT EXISTS products (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  sku VARCHAR(255),
  title TEXT NOT NULL,
  description TEXT,
  category_id UUID,
  attributes JSONB DEFAULT '{}'::jsonb,
  thumbnail_url TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  is_active BOOLEAN DEFAULT TRUE
);
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DO $$ BEGIN
IF NOT EXISTS (
   SELECT 1 FROM pg_trigger WHERE tgname = 'products_set_updated_at') THEN
   CREATE TRIGGER products_set_updated_at
   BEFORE UPDATE ON products
   FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
END IF;
END $$;
`)
    return err
}

func listProducts(w http.ResponseWriter, r *http.Request) {
    // basic pagination
    var products []Product
    if err := db.Select(&products, "SELECT * FROM products ORDER BY created_at DESC LIMIT 100"); err != nil {
        writeError(w, http.StatusInternalServerError, "DB_ERROR", "failed to list products", nil)
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{
        "status": "ok",
        "data": map[string]any{"items": products, "meta": map[string]any{"total": len(products)}},
    })
}

func createProduct(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SKU         string                 `json:"sku"`
        Title       string                 `json:"title"`
        Description string                 `json:"description"`
        CategoryID  *string                `json:"category_id"`
        Attributes  map[string]interface{} `json:"attributes"`
        Thumbnail   *string                `json:"thumbnail"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
        return
    }
    if strings.TrimSpace(req.Title) == "" {
        writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title is required", nil)
        return
    }
    var p Product
    err := db.QueryRowx(`INSERT INTO products (sku,title,description,category_id,attributes,thumbnail_url) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,sku,title,description,category_id,attributes,thumbnail_url,created_at,updated_at,is_active`,
        req.SKU, req.Title, req.Description, req.CategoryID, toJSONB(req.Attributes), req.Thumbnail,
    ).StructScan(&p)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "DB_ERROR", "failed to create product", nil)
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"status": "ok", "data": map[string]any{"product": p}})
}

func toJSONB(m map[string]interface{}) any {
    if m == nil {
        return map[string]interface{}{}
    }
    return m
}

// Auth helpers

func jwtAuthMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            hdr := r.Header.Get("Authorization")
            if hdr == "" || !strings.HasPrefix(hdr, "Bearer ") {
                writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
                return
            }
            tokenStr := strings.TrimPrefix(hdr, "Bearer ")
            claims := jwt.MapClaims{}
            token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
                if verifier == nil {
                    return nil, jwt.ErrTokenUnverifiable
                }
                return verifier, nil
            })
            if err != nil || !token.Valid {
                writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "invalid token", nil)
                return
            }
            roles, _ := claims["roles"].([]interface{})
            ctx := context.WithValue(r.Context(), ctxRolesKey{}, toStringSlice(roles))
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func requireAdmin() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            roles := rolesFromCtx(r.Context())
            if !contains(roles, "admin") {
                writeError(w, http.StatusForbidden, "FORBIDDEN", "admin role required", nil)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// Utils

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "error": map[string]any{"code": code, "message": message, "details": details}})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func getEnv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

// Roles context and helpers

type ctxRolesKey struct{}

func rolesFromCtx(ctx context.Context) []string {
    v := ctx.Value(ctxRolesKey{})
    if v == nil {
        return nil
    }
    ss, _ := v.([]string)
    return ss
}

func toStringSlice(in []interface{}) []string {
    out := make([]string, 0, len(in))
    for _, v := range in {
        if s, ok := v.(string); ok {
            out = append(out, s)
        }
    }
    return out
}

func contains(haystack []string, needle string) bool {
    for _, v := range haystack {
        if v == needle {
            return true
        }
    }
    return false
}

func parseRSAPublicKeyFromPEM(pemBytes []byte) (*rsa.PublicKey, error) {
    block, _ := pem.Decode(pemBytes)
    if block == nil {
        return nil, fmt.Errorf("invalid pem block")
    }
    pkix, err := x509.ParsePKIXPublicKey(block.Bytes)
    if err != nil {
        return nil, err
    }
    k, ok := pkix.(*rsa.PublicKey)
    if !ok {
        return nil, fmt.Errorf("not rsa public key")
    }
    return k, nil
}

