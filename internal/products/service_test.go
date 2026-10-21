package products_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/router"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/validation"
	"github.com/letsssgooo/SOA_DZ_2/internal/products"
)

func TestGeneratedControllerUsesDTOs(t *testing.T) {
	product := openapi.ProductResponse{
		Id: "550e8400-e29b-41d4-a716-446655440000", Name: "Book", Price: 12.5,
		Stock: 0, Category: "books", Status: "ACTIVE",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	service := &dtoService{product: product}
	router := newTestRouter(t, service)
	create := openapi.ProductCreate{
		Name: "Book", Price: 12.5, Stock: 0, Category: "books", Status: "ACTIVE",
	}
	response := request(t, router, http.MethodPost, "/products", create)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body: %s", response.Code, response.Body)
	}
	if !reflect.DeepEqual(service.created, create) {
		t.Fatalf("decoded request = %+v, want %+v", service.created, create)
	}
	var got openapi.ProductResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, product) {
		t.Fatalf("product response = %+v, want %+v", got, product)
	}

	response = request(t, router, http.MethodGet, "/products?status=ACTIVE&category=books", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d; body: %s", response.Code, response.Body)
	}
	var list openapi.GetProducts200Response
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	want := openapi.GetProducts200Response{
		Items: []openapi.ProductResponse{product}, TotalElements: 1, Page: 0, Size: 20,
	}
	if !reflect.DeepEqual(list, want) || service.status != "ACTIVE" || service.category != "books" {
		t.Fatalf("list = %+v, status = %q, category = %q", list, service.status, service.category)
	}
}

type dtoService struct {
	products.Service
	created  openapi.ProductCreate
	product  openapi.ProductResponse
	status   string
	category string
}

func (s *dtoService) CreateProduct(_ context.Context, product openapi.ProductCreate) (openapi.ImplResponse, error) {
	s.created = product
	return openapi.Response(http.StatusCreated, s.product), nil
}

func (s *dtoService) GetProducts(_ context.Context, page, size int32, status, category string) (openapi.ImplResponse, error) {
	s.status, s.category = status, category
	return openapi.Response(http.StatusOK, openapi.GetProducts200Response{
		Items: []openapi.ProductResponse{s.product}, TotalElements: 1, Page: page, Size: size,
	}), nil
}

func request(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func newTestRouter(t *testing.T, service openapi.DefaultAPIServicer) http.Handler {
	t.Helper()
	validator, err := validation.LoadRequestValidator(context.Background(), "../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return router.NewRouter(service, validator)
}
