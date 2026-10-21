package validation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

func TestValidatorUsesConstraintsFromSpecification(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = openapi3.ReadFromFile
	document, err := loader.LoadFromFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	limit := uint64(3)
	document.Paths.Value("/products").Post.RequestBody.Value.Content["application/json"].Schema.Value.Properties["name"].Value.MaxLength = &limit
	validator, err := newRequestValidator(document)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(`{"name":"Book","price":1,"stock":0,"category":"books","status":"ACTIVE"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	validator.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid request reached handler")
	})).ServeHTTP(w, req)
	assertError(t, w, http.StatusBadRequest, openapi.VALIDATION_ERROR)
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
