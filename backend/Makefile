.PHONY: up down rebuild logs auth-shell catalog-shell

COMPOSE_FILE=deployments/docker-compose.yml

up:
	docker compose -f $(COMPOSE_FILE) up --build -d

down:
	docker compose -f $(COMPOSE_FILE) down -v

rebuild:
	docker compose -f $(COMPOSE_FILE) build --no-cache

logs:
	docker compose -f $(COMPOSE_FILE) logs -f --tail=200

auth-shell:
	docker compose -f $(COMPOSE_FILE) exec auth sh -lc "bash || sh"

catalog-shell:
	docker compose -f $(COMPOSE_FILE) exec catalog sh -lc "bash || sh"

