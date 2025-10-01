package main

import (
    "encoding/json"
    "errors"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/golang-jwt/jwt/v5"
    "github.com/google/uuid"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "go.uber.org/zap"
)

type User struct {
    ID           uuid.UUID `json:"id"`
    Email        string    `json:"email"`
    PasswordHash string    `json:"-"`
    FullName     string    `json:"full_name"`
    Role         string    `json:"role"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
    Verified     bool      `json:"verified"`
}

// in-memory store for skeleton
var users = map[string]User{}

type registerRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
    FullName string `json:"full_name"`
}

type loginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

type tokenResponse struct {
    Status       string `json:"status"`
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    User         User   `json:"user"`
}

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

func registerHandler(w http.ResponseWriter, r *http.Request) {
    var req registerRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": map[string]string{"code": "BAD_REQUEST", "message": "invalid body"}})
        return
    }
    email := strings.ToLower(strings.TrimSpace(req.Email))
    if email == "" || req.Password == "" {
        writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": map[string]string{"code": "VALIDATION", "message": "email and password required"}})
        return
    }
    if _, ok := users[email]; ok {
        writeJSON(w, http.StatusConflict, map[string]any{"status": "error", "error": map[string]string{"code": "EMAIL_EXISTS", "message": "email already registered"}})
        return
    }
    u := User{
        ID:        uuid.New(),
        Email:     email,
        FullName:  req.FullName,
        Role:      "buyer",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
        Verified:  true,
    }
    // NOTE: hash omitted in skeleton; do not use in production
    u.PasswordHash = req.Password
    users[email] = u
    writeJSON(w, http.StatusCreated, map[string]any{"status": "ok", "data": map[string]any{"user": u}})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    var req loginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": map[string]string{"code": "BAD_REQUEST", "message": "invalid body"}})
        return
    }
    email := strings.ToLower(strings.TrimSpace(req.Email))
    u, ok := users[email]
    if !ok || u.PasswordHash != req.Password {
        writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "error": map[string]string{"code": "INVALID_CREDENTIALS", "message": "invalid email or password"}})
        return
    }
    access, refresh, err := issueTokens(u)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "error": map[string]string{"code": "TOKEN_ISSUE", "message": "failed to issue token"}})
        return
    }
    writeJSON(w, http.StatusOK, tokenResponse{Status: "ok", AccessToken: access, RefreshToken: refresh, User: u})
}

func issueTokens(u User) (string, string, error) {
    secret := getEnv("JWT_SECRET", "devsecret")
    now := time.Now()
    accessExp := now.Add(15 * time.Minute)
    refreshExp := now.Add(24 * time.Hour)

    accessClaims := jwt.MapClaims{
        "sub": u.ID.String(),
        "email": u.Email,
        "role": u.Role,
        "iat": now.Unix(),
        "exp": accessExp.Unix(),
    }
    refreshClaims := jwt.MapClaims{
        "sub": u.ID.String(),
        "type": "refresh",
        "iat": now.Unix(),
        "exp": refreshExp.Unix(),
    }
    accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
    refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
    a, err := accessToken.SignedString([]byte(secret))
    if err != nil {
        return "", "", err
    }
    r, err := refreshToken.SignedString([]byte(secret))
    if err != nil {
        return "", "", err
    }
    return a, r, nil
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
    var body struct{ RefreshToken string `json:"refresh_token"` }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": map[string]string{"code": "BAD_REQUEST", "message": "invalid body"}})
        return
    }
    secret := getEnv("JWT_SECRET", "devsecret")
    token, err := jwt.Parse(body.RefreshToken, func(token *jwt.Token) (interface{}, error) {
        if token.Method != jwt.SigningMethodHS256 {
            return nil, errors.New("invalid signing method")
        }
        return []byte(secret), nil
    })
    if err != nil || !token.Valid {
        writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "error": map[string]string{"code": "INVALID_TOKEN", "message": "invalid refresh token"}})
        return
    }
    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok || claims["type"] != "refresh" {
        writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "error": map[string]string{"code": "INVALID_TOKEN", "message": "invalid token type"}})
        return
    }
    sub, _ := claims["sub"].(string)
    var u *User
    for _, usr := range users {
        if usr.ID.String() == sub {
            u = &usr
            break
        }
    }
    if u == nil {
        writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "error": map[string]string{"code": "USER_NOT_FOUND", "message": "user not found"}})
        return
    }
    access, refresh, err := issueTokens(*u)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "error": map[string]string{"code": "TOKEN_ISSUE", "message": "failed to issue token"}})
        return
    }
    writeJSON(w, http.StatusOK, tokenResponse{Status: "ok", AccessToken: access, RefreshToken: refresh, User: *u})
}

func meHandler(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "data": map[string]string{"message": "stub"}})
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
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "auth"})
    })
    r.Handle("/metrics", promhttp.Handler())

    r.Route("/api/v1/auth", func(sr chi.Router) {
        sr.Post("/register", registerHandler)
        sr.Post("/login", loginHandler)
        sr.Post("/refresh", refreshHandler)
        sr.Get("/me", meHandler)
    })

    addr := ":" + port
    logger.Info("starting auth-service", zap.String("addr", addr))
    if err := http.ListenAndServe(addr, r); err != nil {
        logger.Fatal("server failed", zap.Error(err))
    }
}

