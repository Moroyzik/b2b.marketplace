Backend Monorepo (Marketplace)

Overview

This repository contains a microservices backend skeleton for a marketplace platform. It includes service scaffolding, Docker-based local development, and initial implementations for authentication and catalog services.

Prerequisites

- Docker and Docker Compose
- Go 1.22+

Quick start

1. Copy env templates and adjust if needed:
   - cp services/auth/.env.example services/auth/.env
   - cp services/catalog/.env.example services/catalog/.env
2. Start infrastructure and services:
   - docker compose -f deployments/docker-compose.yml up --build
3. Auth endpoints (auth-service on :8001):
   - POST /api/v1/auth/register
   - POST /api/v1/auth/login
   - POST /api/v1/auth/refresh
   - GET  /api/v1/auth/me
4. Catalog endpoints (catalog-service on :8002):
   - GET  /api/v1/products
   - POST /api/v1/products (admin-only)

Services

- auth-service: JWT auth with access and refresh tokens, users in Postgres
- catalog-service: products CRUD skeleton (admin create), Postgres

Local development

- Compose file: deployments/docker-compose.yml
- Default DB: postgres://dev:dev@postgres:5432/marketplace?sslmode=disable
- JWT: If no keys are provided via env, auth-service generates a dev RSA key pair on startup. Provide JWT_PUBLIC_KEY to other services (e.g., catalog) to validate tokens.

Migrations

SQL migrations live per service under services/<service>/migrations. You can run them with the service on startup (auto-run) or via a migrate CLI container (Makefile targets will be added later).

Security notes

- Do not commit real secrets. Use .env files locally only.
- In production, provide static RSA keys and rotate regularly.

