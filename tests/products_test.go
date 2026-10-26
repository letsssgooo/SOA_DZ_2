//go:build e2e

package tests

import (
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"
)

// TestProductCRUD проверяет создание, чтение и обновление товара через HTTP и SELECT из БД.
// Проверяет автоматические UUID и даты, очистку пропущенного описания при PUT,
// мягкое удаление со статусом ARCHIVED и повторный DELETE с пустым ответом 204.
func TestProductCRUD(t *testing.T) {
	body := validProduct(newCategory())
	body["description"] = "Исходное описание"
	created := decode[product](t, request(t, "POST", "/products", body, 201))
	if uuid.Validate(created.ID) != nil || created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("UUID и даты должны заполняться автоматически: %+v", created)
	}
	if created.Name != body["name"] || created.Price != 100 || created.Stock != 10 ||
		created.Category != body["category"] || created.Status != "ACTIVE" ||
		created.Description == nil || *created.Description != body["description"] {
		t.Fatalf("Созданный товар отличается от запроса: %+v", created)
	}
	path := "/products/" + created.ID
	got := decode[product](t, request(t, "GET", path, nil, 200))
	assertSameProduct(t, got, created)
	assertSameProduct(t, readDatabaseProduct(t, created.ID), created)

	body["name"], body["price"], body["stock"] = "Обновлённый товар", 150, 7
	body["category"], body["status"] = newCategory(), "INACTIVE"
	delete(body, "description")
	updated := decode[product](t, request(t, "PUT", path, body, 200))
	if updated.ID != created.ID || updated.Name != body["name"] || updated.Price != 150 || updated.Stock != 7 ||
		updated.Category != body["category"] || updated.Status != "INACTIVE" || updated.Description != nil ||
		!updated.CreatedAt.Equal(created.CreatedAt) || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("Неверный результат обновления: %+v", updated)
	}
	assertSameProduct(t, decode[product](t, request(t, "GET", path, nil, 200)), updated)
	assertSameProduct(t, readDatabaseProduct(t, created.ID), updated)

	request(t, "DELETE", path, nil, 204)
	archived := decode[product](t, request(t, "GET", path, nil, 200))
	if archived.Status != "ARCHIVED" || !archived.UpdatedAt.After(updated.UpdatedAt) {
		t.Fatalf("DELETE должен архивировать товар и обновить дату: %+v", archived)
	}
	expected := updated
	expected.Status, expected.UpdatedAt = "ARCHIVED", archived.UpdatedAt
	assertSameProduct(t, archived, expected)
	assertSameProduct(t, readDatabaseProduct(t, created.ID), archived)
	request(t, "DELETE", path, nil, 204)
	repeated := decode[product](t, request(t, "GET", path, nil, 200))
	expected.UpdatedAt = repeated.UpdatedAt
	assertSameProduct(t, repeated, expected)
	t.Log("CRUD: данные сохранены в БД; DELETE оставил строку со статусом ARCHIVED")
}

// assertSameProduct сравнивает все поля товаров, учитывая совпадение дат в разных часовых поясах.
func assertSameProduct(t *testing.T, got, want product) {
	t.Helper()
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("Даты изменились: получили %s / %s, ожидали %s / %s", got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt)
	}
	got.CreatedAt, got.UpdatedAt = want.CreatedAt, want.UpdatedAt
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Получили %+v; ожидали %+v", got, want)
	}
}

// TestProductFiltersAndPagination проверяет фильтры по категории и статусу,
// значения page и size по умолчанию, порядок товаров по UUID и totalElements.
// Проверяет отсутствие пропусков и повторов между страницами и пустые результаты.
// Выводит SELECT созданных товаров обеих категорий и пустой выборки.
func TestProductFiltersAndPagination(t *testing.T) {
	category := newCategory()
	ids := make([]string, 0, 3)
	for _, status := range []string{"ACTIVE", "INACTIVE", "ARCHIVED"} {
		body := validProduct(category)
		body["status"] = status
		created := decode[product](t, request(t, "POST", "/products", body, 201))
		ids = append(ids, created.ID)
	}
	other := decode[product](t, request(t, "POST", "/products", validProduct(newCategory()), 201))
	readDatabaseProduct(t, other.ID)
	readDatabaseProducts(t, category)
	path := "/products?category=" + url.QueryEscape(category)
	all := decode[productList](t, request(t, "GET", path, nil, 200))
	if all.TotalElements != 3 || len(all.Items) != 3 || all.Page != 0 || all.Size != 20 {
		t.Fatalf("Неверные значения списка по умолчанию: %+v", all)
	}
	sort.Strings(ids)
	for i, item := range all.Items {
		if item.ID != ids[i] || item.Category != category {
			t.Fatalf("Неверный порядок или категория: %+v", all)
		}
	}
	for _, status := range []string{"ACTIVE", "INACTIVE", "ARCHIVED"} {
		filtered := decode[productList](t, request(t, "GET", path+"&status="+status, nil, 200))
		if filtered.TotalElements != 1 || len(filtered.Items) != 1 || filtered.Items[0].Status != status || filtered.Items[0].Category != category {
			t.Fatalf("Неверный фильтр %s: %+v", status, filtered)
		}
	}
	for page := 0; page < 3; page++ {
		result := decode[productList](t, request(t, "GET", fmt.Sprintf("%s&page=%d&size=2", path, page), nil, 200))
		wantCount := []int{2, 1, 0}[page]
		if result.TotalElements != 3 || result.Page != page || result.Size != 2 || result.Items == nil || len(result.Items) != wantCount {
			t.Fatalf("Неверная страница %d: %+v", page, result)
		}
		for i, item := range result.Items {
			if item.ID != ids[page*2+i] {
				t.Fatalf("Повтор или пропуск товара на странице %d: %+v", page, result)
			}
		}
	}
	empty := decode[productList](t, request(t, "GET", path+"-missing", nil, 200))
	if empty.Items == nil || len(empty.Items) != 0 || empty.TotalElements != 0 {
		t.Fatalf("Несуществующая категория должна возвращать пустой массив: %+v", empty)
	}
	readDatabaseProducts(t, category+"-missing")
}

// TestDatabaseSchema проверяет через SQL, что миграции Flyway 1–4 успешно применены
// и в таблице products существует действующий индекс по полю status.
func TestDatabaseSchema(t *testing.T) {
	const historyQuery = "SELECT version || ':' || success FROM flyway_schema_history ORDER BY installed_rank;"
	history := sql(t, historyQuery)
	t.Logf("%s\n%s", historyQuery, history)
	if history != "1:true\n2:true\n3:true\n4:true" {
		t.Fatalf("Ожидались успешно применённые миграции 1–4: %s", history)
	}
	const indexQuery = `SELECT EXISTS (
SELECT 1 FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
WHERE i.indrelid = 'products'::regclass AND i.indisvalid AND a.attname = 'status');`
	indexExists := sql(t, indexQuery)
	t.Logf("%s\n%s", indexQuery, indexExists)
	if indexExists != "t" {
		t.Fatal("Нет действующего индекса по status в products")
	}
	t.Log("В PostgreSQL применены миграции Flyway 1–4, индекс по status существует")
}
