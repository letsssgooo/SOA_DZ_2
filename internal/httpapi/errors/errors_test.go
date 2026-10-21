package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

func TestHandleError(t *testing.T) {
	secret := errors.New("SQL driver failure password=secret")
	details := map[string]any{"product_id": "missing"}
	tests := []struct {
		name   string
		err    error
		result *openapi.ImplResponse
		status int
		code   openapi.ErrorCode
		field  string
	}{
		{"required without result", &openapi.RequiredError{Field: "name"}, nil, 400, openapi.VALIDATION_ERROR, "name"},
		{"generated Go field", &openapi.ParsingError{Param: "Price", Err: secret}, nil, 400, openapi.VALIDATION_ERROR, "price"},
		{"typed wrapped", fmt.Errorf("lookup: %w", apierrors.New(openapi.PRODUCT_NOT_FOUND, "Товар не найден", details, secret)), nil, 404, openapi.PRODUCT_NOT_FOUND, ""},
		{"unknown without result", secret, nil, 500, openapi.INTERNAL_SERVER_ERROR, ""},
		{"unknown with result", secret, &openapi.ImplResponse{Code: 404}, 500, openapi.INTERNAL_SERVER_ERROR, ""},
		{"private internal message", apierrors.New(openapi.INTERNAL_SERVER_ERROR, secret.Error(), details, secret), nil, 500, openapi.INTERNAL_SERVER_ERROR, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			HandleError(w, httptest.NewRequest(http.MethodGet, "/products", nil), tt.err, tt.result)
			response := assertError(t, w, tt.status, tt.code)
			if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "driver") {
				t.Fatalf("internal error leaked: %s", w.Body)
			}
			if tt.field != "" {
				fields, ok := response.Details["fields"].(map[string]any)
				if !ok || fields[tt.field] == nil {
					t.Fatalf("missing field %s: %+v", tt.field, response.Details)
				}
			}
			if tt.code == openapi.PRODUCT_NOT_FOUND && !reflect.DeepEqual(response.Details, details) {
				t.Fatalf("details = %+v, want %+v", response.Details, details)
			}
			if tt.status == 500 && (response.Message != internalErrorMessage || response.Details != nil) {
				t.Fatalf("unsafe internal error: %+v", response)
			}
		})
	}
}

func TestRecoveryAfterResponseStarted(t *testing.T) {
	handler := RecoverPanics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial response"))
		panic("late failure")
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusOK || w.Body.String() != "partial response" {
		t.Fatalf("response was rewritten: %d %q", w.Code, w.Body.String())
	}
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
