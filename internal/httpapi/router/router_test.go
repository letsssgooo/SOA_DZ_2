package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/validation"
)

func TestRouterErrors(t *testing.T) {
	tests := []struct {
		name, method, path, allow string
		status                    int
		code                      openapi.ErrorCode
		err                       error
	}{
		{"unknown route", "GET", "/missing", "", 404, openapi.ROUTE_NOT_FOUND, nil},
		{"unsupported item method", "POST", "/products/550e8400-e29b-41d4-a716-446655440000", "DELETE, GET, PUT", 405, openapi.METHOD_NOT_ALLOWED, nil},
		{"read missing product", "GET", "/products/550e8400-e29b-41d4-a716-446655440000", "", 404, openapi.PRODUCT_NOT_FOUND, apierrors.New(openapi.PRODUCT_NOT_FOUND, "Товар не найден", nil, nil)},
		{"unknown service error", "GET", "/products", "", 500, openapi.INTERNAL_SERVER_ERROR, errors.New("private SQL failure")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &testService{respond: func() (openapi.ImplResponse, error) {
				return openapi.Response(200, nil), tt.err
			}}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			newTestRouter(t, service).ServeHTTP(w, req)
			assertError(t, w, tt.status, tt.code)
			if w.Header().Get("Allow") != tt.allow || strings.Contains(w.Body.String(), "private SQL") {
				t.Fatalf("headers = %v, body = %s", w.Header(), w.Body)
			}
		})
	}
}

func TestRouterRecoversServicePanic(t *testing.T) {
	service := &testService{respond: func() (openapi.ImplResponse, error) {
		panic("private driver failure")
	}}
	w := httptest.NewRecorder()
	newTestRouter(t, service).ServeHTTP(w, httptest.NewRequest("GET", "/products", nil))
	assertError(t, w, 500, openapi.INTERNAL_SERVER_ERROR)
	if strings.Contains(w.Body.String(), "driver") {
		t.Fatalf("panic leaked: %s", w.Body)
	}
}

type testService struct {
	respond func() (openapi.ImplResponse, error)
}

func (s *testService) GetProducts(context.Context, int32, int32, string, string) (openapi.ImplResponse, error) {
	return s.respond()
}

func (s *testService) CreateProduct(context.Context, openapi.ProductCreate) (openapi.ImplResponse, error) {
	return s.respond()
}

func (s *testService) GetProductById(context.Context, string) (openapi.ImplResponse, error) {
	return s.respond()
}

func (s *testService) UpdateProduct(context.Context, string, openapi.ProductUpdate) (openapi.ImplResponse, error) {
	return s.respond()
}

func (s *testService) ArchiveProduct(context.Context, string) (openapi.ImplResponse, error) {
	return s.respond()
}

func newTestRouter(t *testing.T, service openapi.DefaultAPIServicer) http.Handler {
	t.Helper()
	validator, err := validation.LoadRequestValidator(context.Background(), "../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(service, validator)
}

func TestRequestValidationBeforeService(t *testing.T) {
	const valid = `{"name":"Book","price":12.5,"stock":0,"category":"books","status":"ACTIVE"}`
	tests := []struct {
		name, body, field string
	}{
		{"name too long", strings.Replace(valid, "Book", strings.Repeat("я", 256), 1), "name"},
		{"category too long", strings.Replace(valid, "books", strings.Repeat("📚", 101), 1), "category"},
		{"description too long", strings.Replace(valid, `"name":`, `"description":"`+strings.Repeat("я", 4001)+`","name":`, 1), "description"},
		{"price before float32 rounding", strings.Replace(valid, "12.5", "0.0099999999", 1), "price"},
		{"fractional stock", strings.Replace(valid, `"stock":0`, `"stock":0.5`, 1), "stock"},
		{"stock overflow", strings.Replace(valid, `"stock":0`, `"stock":2147483648`, 1), "body"},
		{"null stock", strings.Replace(valid, `"stock":0`, `"stock":null`, 1), "stock"},
		{"empty body", "", "body"},
		{"malformed JSON", `{"name":`, "body"},
		{"wrong field type", strings.Replace(valid, `"price":12.5`, `"price":"abc"`, 1), "price"},
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, tt := range tests {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				calls := 0
				service := &testService{respond: func() (openapi.ImplResponse, error) {
					calls++
					return openapi.Response(http.StatusAccepted, nil), nil
				}}
				path := "/products"
				if method == http.MethodPut {
					path += "/550e8400-e29b-41d4-a716-446655440000"
				}
				req := httptest.NewRequest(method, path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				newTestRouter(t, service).ServeHTTP(w, req)
				response := assertError(t, w, http.StatusBadRequest, openapi.VALIDATION_ERROR)
				fields, ok := response.Details["fields"].(map[string]any)
				if calls != 0 || !ok || fields[tt.field] == nil {
					t.Fatalf("calls = %d, details = %+v", calls, response.Details)
				}
				for _, private := range []string{"Schema:", "Value:", ".yaml", "0.0099999999"} {
					if strings.Contains(w.Body.String(), private) {
						t.Fatalf("validation details leaked: %s", w.Body)
					}
				}
			})
		}
	}
}

func TestMultipleValidationErrorsBeforeService(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, body string
		fields                   []string
	}{
		{"create", http.MethodPost, "/products", `{"name":"","price":0,"stock":-1,"category":"","status":"UNKNOWN"}`, []string{"name", "price", "stock", "category", "status"}},
		{"update", http.MethodPut, "/products/550e8400-e29b-41d4-a716-446655440000", `{"name":"","price":0,"stock":-1,"category":"","status":"UNKNOWN"}`, []string{"name", "price", "stock", "category", "status"}},
		{"query", http.MethodGet, "/products?page=-1&size=0&status=UNKNOWN", "", []string{"page", "size", "status"}},
		{"missing update fields", http.MethodPut, "/products/550e8400-e29b-41d4-a716-446655440000", `{}`, []string{"name", "price", "stock", "category", "status"}},
		{"missing create fields", http.MethodPost, "/products", `{}`, []string{"name", "price", "stock", "category", "status"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			service := &testService{respond: func() (openapi.ImplResponse, error) {
				calls++
				return openapi.Response(http.StatusAccepted, nil), nil
			}}
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			newTestRouter(t, service).ServeHTTP(w, req)
			response := assertError(t, w, http.StatusBadRequest, openapi.VALIDATION_ERROR)
			fields, ok := response.Details["fields"].(map[string]any)
			if calls != 0 || !ok || len(fields) != len(tt.fields) {
				t.Fatalf("calls = %d, details = %+v", calls, response.Details)
			}
			for _, field := range tt.fields {
				messages, ok := fields[field].([]any)
				if !ok || len(messages) != 1 || messages[0] == "" {
					t.Fatalf("missing or duplicate violations for %s: %+v", field, fields)
				}
			}
		})
	}
}

func TestRequestValidationPathAndQuery(t *testing.T) {
	for _, tt := range []struct{ path, field string }{
		{"/products/not-a-uuid", "id"},
		{"/products?page=abc", "page"},
		{"/products?size=2147483648", "size"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			service := &testService{respond: func() (openapi.ImplResponse, error) {
				t.Fatal("invalid request reached service")
				return openapi.ImplResponse{}, nil
			}}
			w := httptest.NewRecorder()
			newTestRouter(t, service).ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
			response := assertError(t, w, http.StatusBadRequest, openapi.VALIDATION_ERROR)
			fields, ok := response.Details["fields"].(map[string]any)
			if !ok || fields[tt.field] == nil {
				t.Fatalf("missing field %s: %+v", tt.field, response.Details)
			}
		})
	}
}

func TestValidatedBodyReachesGeneratedController(t *testing.T) {
	name, description, category := strings.Repeat("я", 255), strings.Repeat("📚", 4000), strings.Repeat("я", 100)
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, state := range []string{"missing", "null", "value"} {
			t.Run(method+"/"+state, func(t *testing.T) {
				service := &validationDTOService{}
				body := map[string]any{"name": name, "price": 0.01, "stock": 0, "category": category, "status": "ACTIVE", "extra": true}
				if state == "null" {
					body["description"] = nil
				} else if state == "value" {
					body["description"] = description
				}
				data, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				path := "/products"
				if method == http.MethodPut {
					path += "/550e8400-e29b-41d4-a716-446655440000"
				}
				req := httptest.NewRequest(method, path, strings.NewReader(string(data)))
				req.Header.Set("Content-Type", "application/json; charset=utf-8")
				w := httptest.NewRecorder()
				newTestRouter(t, service).ServeHTTP(w, req)
				if w.Code != http.StatusAccepted || service.calls != 1 {
					t.Fatalf("status = %d, calls = %d, body = %s", w.Code, service.calls, w.Body)
				}
				got := service.product
				if got.Name != name || got.Category != category || got.Price != float32(0.01) || got.Stock != 0 {
					t.Fatalf("validated DTO changed: %+v", got)
				}
				if state == "value" {
					if got.Description == nil || *got.Description != description {
						t.Fatal("description changed")
					}
				} else if got.Description != nil {
					t.Fatal("absent or null description must remain nil")
				}
			})
		}
	}
}

type validationDTOService struct {
	openapi.DefaultAPIServicer
	calls   int
	product openapi.ProductCreate
}

func (s *validationDTOService) CreateProduct(_ context.Context, product openapi.ProductCreate) (openapi.ImplResponse, error) {
	s.calls++
	s.product = product
	return openapi.Response(http.StatusAccepted, nil), nil
}

func (s *validationDTOService) UpdateProduct(ctx context.Context, _ string, product openapi.ProductUpdate) (openapi.ImplResponse, error) {
	return s.CreateProduct(ctx, openapi.ProductCreate(product))
}

func assertError(t *testing.T, w *httptest.ResponseRecorder, status int, code openapi.ErrorCode) openapi.ErrorResponse {
	t.Helper()
	if w.Code != status || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status = %d, headers = %v, body = %s", w.Code, w.Header(), w.Body)
	}
	var response openapi.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid error JSON: %v; body = %s", err, w.Body)
	}
	if response.ErrorCode != code || response.Message == "" {
		t.Fatalf("error = %+v, want code %s", response, code)
	}
	return response
}
