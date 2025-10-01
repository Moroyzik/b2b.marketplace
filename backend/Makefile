SHELL := /usr/bin/bash

.PHONY: up down build rebuild logs ps clean

up:
	docker-compose up --build

down:
	docker-compose down -v

build:
	docker-compose build

rebuild:
	docker-compose build --no-cache

logs:
	docker-compose logs -f --tail=200

ps:
	docker-compose ps

clean: down
	docker system prune -f

