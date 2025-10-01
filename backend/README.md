## Marketplace Backend (Monorepo Skeleton)

This directory contains a microservices skeleton for a marketplace backend, intended for local development via Docker Compose.

### Services (initial)

- api-gateway (REST proxy)
- auth-service (auth skeleton)
- catalog-service (products skeleton)
- offer-service (offers skeleton)
- cart-service (cart skeleton)
- order-service (orders skeleton)
- search-service (search skeleton)

Infra dependencies (dev): PostgreSQL, Redis, RabbitMQ, MinIO, Elasticsearch.

### Quick start

1. Ensure Docker and Docker Compose are installed.
2. From this `backend` directory, run:

```bash
docker-compose up --build
```

3. Services will be available on localhost ports as defined in `docker-compose.yml`.

### Default endpoints

- Gateway: http://localhost:8080/health
- Auth: http://localhost:8001/health
- Catalog: http://localhost:8002/health
- Offer: http://localhost:8003/health
- Search: http://localhost:8004/health
- Cart: http://localhost:8005/health
- Order: http://localhost:8006/health

Each service also exposes `/metrics` (stub) and minimal API placeholders.

### Environment

Each service contains a `.env.example`. Copy to `.env` and adjust as needed (for dev defaults should work out of the box).

### Migrations

SQL migrations are placed under each service's `migrations/`. They are not automatically applied in this skeleton. Use your preferred migration tool (e.g., golang-migrate) or integrate a migrator service.

### Notes

- This is a starter skeleton focusing on structure and local run-ability.
- Replace stub handlers with full implementations incrementally.

