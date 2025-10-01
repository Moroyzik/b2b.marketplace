package main

import (
    "context"
    "crypto/rand"
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
    "golang.org/x/crypto/bcrypt"
)

type AppConfig struct {
    Port              string
    DatabaseURL       string
    RedisAddr         string
    AccessTokenTTL    time.Duration
    RefreshTokenTTL   time.Duration
    BcryptCost        int
    PrivateKeyPEM     string
    PublicKeyPEM      string
}

type User struct {
    ID         string    `db:"id" json:"id"`
    Email      string    `db:"email" json:"email"`
    Password   string    `db:"password_hash" json:"-"`
    FullName   string    `db:"full_name" json:"full_name"`
    Role       string    `db:"role" json:"role"`
    CreatedAt  time.Time `db:"created_at" json:"created_at"`
    UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
    IsActive   bool      `db:"is_active" json:"is_active"`
    Verified   bool      `db:"verified" json:"verified"`
}

type RegisterRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
    FullName string `json:"full_name"`
}

type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

type TokenResponse struct {
    Status       string      `json:"status"`
    Data         interface{} `json:"data,omitempty"`
    Error        *APIError   `json:"error,omitempty"`
}

type APIError struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

type TokenPair struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    User         User   `json:"user"`
}

var (
    db     *sqlx.DB
    signer *rsa.PrivateKey
    verifier *rsa.PublicKey
)

func loadConfig() (AppConfig, error) {
    cfg := AppConfig{
        Port:            getEnv("PORT", "8001"),
        DatabaseURL:     getEnv("DATABASE_URL", "postgres://dev:dev@localhost:5432/marketplace?sslmode=disable"),
        RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
        AccessTokenTTL:  getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
        RefreshTokenTTL: getEnvDuration("REFRESH_TOKEN_TTL", 24*time.Hour*7),
    }
    cost := 12
    if v := os.Getenv("BCRYPT_COST"); v != "" {
        if n, err := parseInt(v); err == nil && n >= 4 && n <= 15 {
            cost = n
        }
    }
    cfg.BcryptCost = cost
    cfg.PrivateKeyPEM = os.Getenv("JWT_PRIVATE_KEY")
    cfg.PublicKeyPEM = os.Getenv("JWT_PUBLIC_KEY")
    return cfg, nil
}

func main() {
    cfg, err := loadConfig()
    if err != nil {
        log.Fatalf("config error: %v", err)
    }

    // Setup RSA keys
    if cfg.PrivateKeyPEM == "" || cfg.PublicKeyPEM == "" {
        // generate dev keypair
        k, err := rsa.GenerateKey(rand.Reader, 2048)
        if err != nil {
            log.Fatalf("rsa key gen: %v", err)
        }
        signer = k
        verifier = &k.PublicKey
        // print public key for other services
        pubDer, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
        if err != nil {
            log.Fatalf("marshal public key: %v", err)
        }
        pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDer})
        log.Printf("[auth] Generated dev RSA keys. Provide JWT_PUBLIC_KEY to other services.\n%s", string(pubPEM))
    } else {
        // parse provided keys
        privBlock, _ := pem.Decode([]byte(cfg.PrivateKeyPEM))
        if privBlock == nil {
            log.Fatalf("invalid JWT_PRIVATE_KEY PEM")
        }
        pk, err := x509.ParsePKCS1PrivateKey(privBlock.Bytes)
        if err != nil {
            log.Fatalf("parse private key: %v", err)
        }
        signer = pk

        pubBlock, _ := pem.Decode([]byte(cfg.PublicKeyPEM))
        if pubBlock == nil {
            log.Fatalf("invalid JWT_PUBLIC_KEY PEM")
        }
        parsed, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
        if err != nil {
            log.Fatalf("parse public key: %v", err)
        }
        rp, ok := parsed.(*rsa.PublicKey)
        if !ok {
            log.Fatalf("public key is not RSA")
        }
        verifier = rp
    }

    // DB
    db, err = sqlx.Connect("postgres", cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("db connect: %v", err)
    }
    if err := ensureMigrations(db.DB); err != nil {
        log.Fatalf("migrations: %v", err)
    }

    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    r.Post("/api/v1/auth/register", handleRegister(cfg))
    r.Post("/api/v1/auth/login", handleLogin(cfg))
    r.Post("/api/v1/auth/refresh", handleRefresh(cfg))
    r.Group(func(protected chi.Router) {
        protected.Use(authMiddleware())
        protected.Get("/api/v1/auth/me", handleMe)
        protected.Post("/api/v1/auth/logout", handleLogout)
    })

    addr := ":" + cfg.Port
    log.Printf("auth-service listening on %s", addr)
    if err := http.ListenAndServe(addr, r); err != nil {
        log.Fatalf("server error: %v", err)
    }
}

func ensureMigrations(sqldb *sql.DB) error {
    _, err := sqldb.Exec(`
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  full_name VARCHAR(255),
  role VARCHAR(32) NOT NULL DEFAULT 'buyer',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  is_active BOOLEAN DEFAULT TRUE,
  verified BOOLEAN DEFAULT FALSE
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
   SELECT 1 FROM pg_trigger WHERE tgname = 'users_set_updated_at') THEN
   CREATE TRIGGER users_set_updated_at
   BEFORE UPDATE ON users
   FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
END IF;
END $$;
`)
    return err
}

func handleRegister(cfg AppConfig) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req RegisterRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
            return
        }
        req.Email = strings.TrimSpace(strings.ToLower(req.Email))
        if req.Email == "" || len(req.Password) < 6 {
            writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "email and password required (min 6)", nil)
            return
        }
        var exists int
        if err := db.Get(&exists, "SELECT 1 FROM users WHERE email=$1", req.Email); err == nil {
            writeError(w, http.StatusConflict, "EMAIL_TAKEN", "email already registered", nil)
            return
        }
        hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), cfg.BcryptCost)
        if err != nil {
            writeError(w, http.StatusInternalServerError, "HASH_ERROR", "could not process password", nil)
            return
        }
        var user User
        err = db.QueryRowx(`INSERT INTO users (email, password_hash, full_name) VALUES ($1,$2,$3) RETURNING id,email,password_hash,full_name,role,created_at,updated_at,is_active,verified`, req.Email, string(hash), req.FullName).StructScan(&user)
        if err != nil {
            writeError(w, http.StatusInternalServerError, "DB_ERROR", "could not create user", nil)
            return
        }
        writeJSON(w, http.StatusCreated, map[string]any{
            "status": "ok",
            "data": map[string]any{"user": user},
        })
    }
}

func handleLogin(cfg AppConfig) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req LoginRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
            return
        }
        var user User
        if err := db.Get(&user, "SELECT * FROM users WHERE email=$1", strings.ToLower(req.Email)); err != nil {
            writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password", nil)
            return
        }
        if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
            writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password", nil)
            return
        }
        access, refresh, err := mintTokenPair(user, cfg)
        if err != nil {
            writeError(w, http.StatusInternalServerError, "TOKEN_ERROR", "could not create token", nil)
            return
        }
        writeJSON(w, http.StatusOK, TokenResponse{Status: "ok", Data: TokenPair{AccessToken: access, RefreshToken: refresh, User: user}})
    }
}

func handleRefresh(cfg AppConfig) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var payload struct{ RefreshToken string `json:"refresh_token"` }
        if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.RefreshToken == "" {
            writeError(w, http.StatusBadRequest, "BAD_REQUEST", "refresh_token required", nil)
            return
        }
        token, claims, err := parseAndValidate(payload.RefreshToken)
        if err != nil || !token.Valid {
            writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "invalid refresh token", nil)
            return
        }
        if claims["typ"] != "refresh" {
            writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "invalid token type", nil)
            return
        }
        userID, _ := claims["sub"].(string)
        var user User
        if err := db.Get(&user, "SELECT * FROM users WHERE id=$1", userID); err != nil {
            writeError(w, http.StatusUnauthorized, "USER_NOT_FOUND", "user not found", nil)
            return
        }
        access, refresh, err := mintTokenPair(user, cfg)
        if err != nil {
            writeError(w, http.StatusInternalServerError, "TOKEN_ERROR", "could not create token", nil)
            return
        }
        writeJSON(w, http.StatusOK, TokenResponse{Status: "ok", Data: TokenPair{AccessToken: access, RefreshToken: refresh, User: user}})
    }
}

func handleMe(w http.ResponseWriter, r *http.Request) {
    userID := r.Context().Value(ctxUserIDKey{}).(string)
    var user User
    if err := db.Get(&user, "SELECT * FROM users WHERE id=$1", userID); err != nil {
        writeError(w, http.StatusNotFound, "USER_NOT_FOUND", "user not found", nil)
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "data": map[string]any{"user": user}})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
    // For opaque refresh token store, revoke here. With stateless refresh, instruct client to delete.
    writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(TokenResponse{Status: "error", Error: &APIError{Code: code, Message: message, Details: details}})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func mintTokenPair(user User, cfg AppConfig) (string, string, error) {
    now := time.Now()
    accessClaims := jwt.MapClaims{
        "sub":   user.ID,
        "email": user.Email,
        "roles": []string{user.Role},
        "iat":   now.Unix(),
        "exp":   now.Add(cfg.AccessTokenTTL).Unix(),
        "typ":   "access",
    }
    accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
    accessStr, err := accessToken.SignedString(signer)
    if err != nil {
        return "", "", err
    }
    refreshClaims := jwt.MapClaims{
        "sub": user.ID,
        "iat": now.Unix(),
        "exp": now.Add(cfg.RefreshTokenTTL).Unix(),
        "typ": "refresh",
    }
    refreshToken := jwt.NewWithClaims(jwt.SigningMethodRS256, refreshClaims)
    refreshStr, err := refreshToken.SignedString(signer)
    if err != nil {
        return "", "", err
    }
    return accessStr, refreshStr, nil
}

func parseAndValidate(tokenStr string) (*jwt.Token, jwt.MapClaims, error) {
    claims := jwt.MapClaims{}
    token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
        if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
        }
        return verifier, nil
    })
    return token, claims, err
}

type ctxUserIDKey struct{}

func authMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            hdr := r.Header.Get("Authorization")
            if hdr == "" || !strings.HasPrefix(hdr, "Bearer ") {
                writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
                return
            }
            tokenStr := strings.TrimPrefix(hdr, "Bearer ")
            token, claims, err := parseAndValidate(tokenStr)
            if err != nil || !token.Valid {
                writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "invalid token", nil)
                return
            }
            sub, _ := claims["sub"].(string)
            if sub == "" {
                writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "missing sub", nil)
                return
            }
            ctx := context.WithValue(r.Context(), ctxUserIDKey{}, sub)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func getEnv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func getEnvDuration(key string, d time.Duration) time.Duration {
    if v := os.Getenv(key); v != "" {
        if parsed, err := time.ParseDuration(v); err == nil {
            return parsed
        }
    }
    return d
}

func parseInt(s string) (int, error) {
    var n int
    _, err := fmt.Sscan(s, &n)
    if err != nil {
        return 0, err
    }
    return n, nil
}

