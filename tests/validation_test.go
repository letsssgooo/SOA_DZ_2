//go:build e2e

package tests

import (
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
)

// TestProductValidation проверяет отклонение невалидных POST и PUT: обязательные поля,
// null, неверные типы, длины строк, цену, остаток, статус и некорректный JSON.
// Проверяет ответ 400 с ошибками полей, отсутствие созданных товаров после ошибочных POST
// и неизменность товара, включая даты, после каждого ошибочного PUT.
// Выводит SELECT пустой категории после POST и сохранённого товара до и после невалидных PUT.
func TestProductValidation(t *testing.T) {
	type invalidCase struct {
		name, field string
		value       any
		omit        bool
		fields      []string
	}
	cases := []invalidCase{
		{name: "empty name", field: "name", value: ""},
		{name: "long Unicode name", field: "name", value: strings.Repeat("я", 256)},
		{name: "long Unicode description", field: "description", value: strings.Repeat("📚", 4001)},
		{name: "empty category", field: "category", value: ""},
		{name: "long Unicode category", field: "category", value: strings.Repeat("я", 101)},
		{name: "price below minimum", field: "price", value: 0.0099999999},
		{name: "negative price", field: "price", value: -1},
		{name: "negative stock", field: "stock", value: -1},
		{name: "fractional stock", field: "stock", value: 0.5},
		{name: "stock overflow", field: "stock", value: int64(2147483648), fields: []string{"body"}},
		{name: "unknown status", field: "status", value: "UNKNOWN"},
		{name: "name type", field: "name", value: 1},
		{name: "description type", field: "description", value: true},
		{name: "price type", field: "price", value: "100"},
		{name: "stock type", field: "stock", value: "10"},
		{name: "category type", field: "category", value: []string{"books"}},
		{name: "status type", field: "status", value: false},
		{name: "empty body", value: json.RawMessage(""), fields: []string{"body"}},
		{name: "malformed JSON", value: json.RawMessage(`{"name":`), fields: []string{"body"}},
		{name: "array body", value: json.RawMessage(`[]`), fields: []string{"body"}},
		{name: "missing all fields", value: map[string]any{}, fields: []string{"name", "price", "stock", "category", "status"}},
		{name: "multiple invalid fields", value: map[string]any{"name": "", "price": 0, "stock": -1, "category": "", "status": "UNKNOWN"}, fields: []string{"name", "price", "stock", "category", "status"}},
	}
	for _, field := range []string{"name", "price", "stock", "category", "status"} {
		cases = append(cases,
			invalidCase{name: "missing " + field, field: field, omit: true},
			invalidCase{name: "null " + field, field: field, value: nil},
		)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			category := newCategory()
			path := "/products"
			var original product
			if method == http.MethodPut {
				original = decode[product](t, request(t, http.MethodPost, path, validProduct(category), 201))
				assertSameProduct(t, readDatabaseProduct(t, original.ID), original)
				path += "/" + original.ID
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					var body any = tc.value
					fields := tc.fields
					if tc.field != "" {
						payload := validProduct(category)
						payload[tc.field] = tc.value
						if tc.omit {
							delete(payload, tc.field)
						}
						body = payload
						if fields == nil {
							fields = []string{tc.field}
						}
					}
					checkAPIError(t, request(t, method, path, body, 400), "VALIDATION_ERROR", fields...)
					if method == http.MethodPut {
						got := decode[product](t, request(t, http.MethodGet, path, nil, 200))
						if !reflect.DeepEqual(got, original) {
							t.Fatalf("rejected PUT changed product: got %+v, want %+v", got, original)
						}
						assertSameProduct(t, readDatabaseProduct(t, original.ID), original)
					}
				})
			}
			if method == http.MethodPost {
				page := decode[struct {
					Items         []product `json:"items"`
					TotalElements int       `json:"totalElements"`
				}](t, request(t, http.MethodGet, "/products?category="+url.QueryEscape(category), nil, 200))
				if page.TotalElements != 0 || len(page.Items) != 0 {
					t.Fatalf("rejected POST created products: %+v", page)
				}
				if products := readDatabaseProducts(t, category); len(products) != 0 {
					t.Fatalf("Невалидные POST создали записи в БД: %+v", products)
				}
			}
		})
	}
}

// TestValidProductBoundaries проверяет допустимые границы POST и PUT: Unicode-строки
// максимальной длины, цену 0.01, нулевой остаток и дополнительные поля.
// Проверяет максимальное, null и пропущенное описание, а также сохранение значений через GET.
// Цена сравнивается с допуском из-за использования float32 в сгенерированных DTO.
// Выводит SELECT исходного товара для PUT и сохранённых граничных значений.
func TestValidProductBoundaries(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, descriptionState := range []string{"maximum", "null", "omitted"} {
			t.Run(method+"/description_"+descriptionState, func(t *testing.T) {
				category := newCategory()
				category = strings.Repeat("я", 100-utf8.RuneCountInString(category)) + category
				payload := validProduct(category)
				payload["name"] = strings.Repeat("я", 255)
				payload["price"] = 0.01
				payload["stock"] = 0
				payload["extra"] = "allowed by OpenAPI"
				if descriptionState == "maximum" {
					payload["description"] = strings.Repeat("📚", 4000)
				} else if descriptionState == "null" {
					payload["description"] = nil
				}
				path, status := "/products", 201
				if method == http.MethodPut {
					initial := validProduct(category)
					initial["description"] = "This description must be replaced or cleared"
					created := decode[product](t, request(t, http.MethodPost, path, initial, 201))
					readDatabaseProduct(t, created.ID)
					path += "/" + created.ID
					status = 200
				}
				got := decode[product](t, request(t, method, path, payload, status))
				readDatabaseProduct(t, got.ID)
				if got.Name != payload["name"] || got.Category != category || got.Stock != 0 || got.Status != "ACTIVE" {
					t.Fatalf("boundary values changed: %+v", got)
				}
				if math.Abs(got.Price-0.01) > 1e-8 {
					t.Fatalf("price = %v, want approximately 0.01", got.Price)
				}
				if descriptionState == "maximum" {
					if got.Description == nil || *got.Description != payload["description"] {
						t.Fatal("maximum Unicode description was changed")
					}
				} else if got.Description != nil {
					t.Fatalf("%s description = %q, want nil", descriptionState, *got.Description)
				}
				stored := decode[product](t, request(t, http.MethodGet, "/products/"+got.ID, nil, 200))
				if !reflect.DeepEqual(stored, got) {
					t.Fatalf("GET differs from saved product: got %+v, want %+v", stored, got)
				}
			})
		}
	}
}

// TestPathAndQueryValidation проверяет ответ 400 с указанием ошибочного поля
// для невалидного UUID в GET, PUT и DELETE, а также неверных page, size и status.
func TestPathAndQueryValidation(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method+"/invalid_uuid", func(t *testing.T) {
			var body any
			if method == http.MethodPut {
				body = validProduct(newCategory())
			}
			checkAPIError(t, request(t, method, "/products/not-a-uuid", body, 400), "VALIDATION_ERROR", "id")
		})
	}
	for _, tc := range []struct {
		query  string
		fields []string
	}{
		{"page=-1", []string{"page"}},
		{"page=abc", []string{"page"}},
		{"page=1.5", []string{"page"}},
		{"page=2147483648", []string{"page"}},
		{"size=0", []string{"size"}},
		{"size=-1", []string{"size"}},
		{"size=abc", []string{"size"}},
		{"size=1.5", []string{"size"}},
		{"size=2147483648", []string{"size"}},
		{"status=UNKNOWN", []string{"status"}},
		{"page=-1&size=0&status=UNKNOWN", []string{"page", "size", "status"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			checkAPIError(t, request(t, http.MethodGet, "/products?"+tc.query, nil, 400), "VALIDATION_ERROR", tc.fields...)
		})
	}
}

// TestHTTPErrorResponses проверяет ответы 404 для отсутствующего товара и неизвестного пути,
// ответы 405 с заголовком Allow для неподдерживаемых методов и единый формат ошибок.
func TestHTTPErrorResponses(t *testing.T) {
	missingPath := "/products/" + uuid.NewString()
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method+"/not_found", func(t *testing.T) {
			var body any
			if method == http.MethodPut {
				body = validProduct(newCategory())
			}
			checkAPIError(t, request(t, method, missingPath, body, 404), "PRODUCT_NOT_FOUND")
		})
	}
	checkAPIError(t, request(t, http.MethodGet, "/missing/"+newCategory(), nil, 404), "ROUTE_NOT_FOUND")
	for _, tc := range []struct{ method, path, allow string }{
		{http.MethodPatch, "/products", "GET, POST"},
		{http.MethodPost, missingPath, "DELETE, GET, PUT"},
	} {
		t.Run(tc.method+"/method_not_allowed", func(t *testing.T) {
			resp := request(t, tc.method, tc.path, nil, 405)
			checkAPIError(t, resp, "METHOD_NOT_ALLOWED")
			if got := resp.Header.Get("Allow"); got != tc.allow {
				t.Fatalf("Allow = %q, want %q", got, tc.allow)
			}
		})
	}
}

// checkAPIError проверяет код ошибки, сообщение, ошибки полей и отсутствие служебных фрагментов.
func checkAPIError(t *testing.T, resp response, code string, fields ...string) {
	t.Helper()
	got := decode[struct {
		Code    string `json:"error_code"`
		Message string `json:"message"`
		Details struct {
			Fields map[string][]string `json:"fields"`
		} `json:"details"`
	}](t, resp)
	if got.Code != code || strings.TrimSpace(got.Message) == "" {
		t.Fatalf("error = %+v, want code %s and a nonempty message", got, code)
	}
	for _, field := range fields {
		messages := got.Details.Fields[field]
		if len(messages) == 0 || strings.TrimSpace(messages[0]) == "" {
			t.Errorf("no validation message for %q: %+v", field, got.Details.Fields)
		}
	}
	for _, private := range []string{"Schema:", "Value:", ".yaml"} {
		if strings.Contains(string(resp.Body), private) {
			t.Errorf("error exposes internal details: %s", resp.Body)
		}
	}
}
