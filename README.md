Сервис маркетплейса на Go для создания, получения, обновления и архивации товаров.
API описано в OpenAPI, обработчики и DTO создаются с помощью OpenAPI Generator.
Данные хранятся в PostgreSQL, миграции применяются через Flyway.

Основные команды Make:

```bash
make up                # собрать и запустить приложение и БД
make stop              # остановить контейнеры, сохранив данные
make logs              # смотреть только новые JSON-логи
make test              # обычные Go-тесты
make test-e2e          # все E2E-тесты при запущенных контейнерах
make validate-openapi  # проверить спецификацию
make generate          # сгенерировать Go-код из OpenAPI
make build             # сгенерировать код и собрать приложение
make migrate           # применить миграции Flyway
```

Запуск одного E2E-теста при запущенном сервисе — замени `ИМЯ_ТЕСТА`:

```bash
go test -tags=e2e -v -count=1 ./tests -run '^ИМЯ_ТЕСТА$'
```

Вход в SQL-консоль PostgreSQL из папки проекта:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
```

В консоли вводи SQL-запросы с `;` в конце. Выход — `\q`.
