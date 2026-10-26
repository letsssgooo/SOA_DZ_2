//go:build e2e

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	baseURL             = "http://localhost:8080"
	maxJSONOutputLength = 600
)

var (
	projectDir string
	client     = &http.Client{Timeout: 5 * time.Second}
)

type response struct {
	Header http.Header
	Body   []byte
}

type product struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type productList struct {
	Items         []product `json:"items"`
	TotalElements int       `json:"totalElements"`
	Page          int       `json:"page"`
	Size          int       `json:"size"`
}

// TestMain ожидает готовности отдельно запущенного API до 30 секунд и запускает E2E-тесты.
func TestMain(m *testing.M) {
	var err error
	projectDir, err = filepath.Abs("..")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/products?size=1", nil)
		res, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				fmt.Println("E2E: HTTP → сервис → PostgreSQL; адрес " + baseURL)
				os.Exit(m.Run())
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "Сервис недоступен. Запустите docker compose up --build -d и проверьте docker compose logs app flyway")
	os.Exit(1)
}

// newCategory создаёт уникальную категорию, чтобы данные разных тестов не смешивались.
func newCategory() string { return "e2e-" + uuid.NewString() }

// validProduct возвращает корректное тело запроса с заданной категорией.
func validProduct(category string) map[string]any {
	return map[string]any{
		"name": "Тестовый товар", "price": 100, "stock": 10,
		"category": category, "status": "ACTIVE",
	}
}

// request отправляет HTTP-запрос, выводит ответ и проверяет статус, JSON и X-Request-Id.
// RawMessage отправляет без преобразования; созданные товары остаются в БД после теста.
func request(t *testing.T, method, path string, body any, wantStatus int) response {
	t.Helper()
	var data []byte
	var err error
	if raw, ok := body.(json.RawMessage); ok {
		data = raw
	} else if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s %s → %d; X-Request-Id: %s\nОтвет:\n%s", method, path, res.StatusCode, res.Header.Get("X-Request-Id"), shortJSON(data))
	if res.StatusCode != wantStatus {
		t.Fatalf("HTTP-статус: получили %d, ожидали %d; тело: %s", res.StatusCode, wantStatus, data)
	}
	if err := uuid.Validate(res.Header.Get("X-Request-Id")); err != nil {
		t.Fatalf("X-Request-Id должен содержать UUID: %v", err)
	}
	if wantStatus == http.StatusNoContent {
		if len(data) != 0 {
			t.Fatalf("Ответ 204 должен быть пустым: %s", data)
		}
	} else if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") || !json.Valid(data) {
		t.Fatalf("Ожидался JSON: Content-Type=%q, body=%s", res.Header.Get("Content-Type"), data)
	}
	return response{Header: res.Header, Body: data}
}

// decode разбирает JSON-ответ в тип T и завершает тест с ошибкой при неудаче.
func decode[T any](t *testing.T, res response) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(res.Body, &value); err != nil {
		t.Fatalf("Разбор JSON: %v; тело: %s", err, res.Body)
	}
	return value
}

// shortJSON форматирует JSON с отступами и сокращает вывод до maxJSONOutputLength символов.
func shortJSON(data []byte) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err == nil {
		data = pretty.Bytes()
	}
	text := []rune(strings.TrimSpace(string(data)))
	if len(text) > maxJSONOutputLength {
		return string(text[:maxJSONOutputLength]) + "… [вывод сокращён]"
	}
	return string(text)
}

// compose выполняет команду Docker Compose, передаёт input на стандартный ввод и возвращает вывод.
func compose(t *testing.T, input string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Dir = projectDir
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// sql выполняет SQL через psql в контейнере PostgreSQL, подставляя переданные переменные psql.
func sql(t *testing.T, query string, variables ...string) string {
	t.Helper()
	args := []string{"exec", "-T", "postgres", "sh", "-c",
		`exec psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"`, "sh"}
	for _, variable := range variables {
		args = append(args, "-v", variable)
	}
	return compose(t, query, args...)
}

// readDatabaseProduct читает товар напрямую через SELECT и выводит результат для сравнения с API.
func readDatabaseProduct(t *testing.T, id string) product {
	t.Helper()
	const query = "SELECT row_to_json(p) FROM products p WHERE id = :'id';"
	data := sql(t, query, "id="+id)
	t.Logf("%s\nid=%s\n%s", query, id, shortJSON([]byte(data)))
	if data == "" {
		t.Fatalf("Товар %s отсутствует в PostgreSQL", id)
	}
	return decode[product](t, response{Body: []byte(data)})
}

// readDatabaseProducts выводит SELECT товаров указанной категории; при отсутствии записей показывает [].
func readDatabaseProducts(t *testing.T, category string) []product {
	t.Helper()
	const query = "SELECT row_to_json(p) FROM products p WHERE category = :'category' ORDER BY id;"
	data := sql(t, query, "category="+category)
	t.Logf("%s\ncategory=%s", query, category)
	products := make([]product, 0)
	if data == "" {
		t.Log("Результат: []")
		return products
	}
	for _, row := range strings.Split(data, "\n") {
		t.Logf("%s", shortJSON([]byte(row)))
		products = append(products, decode[product](t, response{Body: []byte(row)}))
	}
	return products
}
