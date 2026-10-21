FROM openapitools/openapi-generator-cli:v7.26.0 AS codegen
WORKDIR /app
COPY api ./api
RUN docker-entrypoint.sh generate -c /app/api/codegen.yaml

FROM golang:1.26.5-alpine
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=codegen /app/internal/gen/openapi ./internal/gen/openapi
COPY api ./api
RUN go build -o /app/server ./cmd/server

EXPOSE 8080
ENTRYPOINT ["/app/server"]
