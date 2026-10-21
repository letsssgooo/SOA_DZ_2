package apierrors_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

func TestErrorWrapping(t *testing.T) {
	cause := errors.New("driver failure: SELECT secret FROM internal_table; password=private")
	appError := apierrors.New(openapi.PRODUCT_NOT_FOUND, "Товар не найден", nil, cause)
	wrapped := fmt.Errorf("get product: %w", appError)
	if !errors.Is(wrapped, cause) || errors.Unwrap(appError) != cause {
		t.Fatal("original cause must remain in the error chain")
	}
	var got *apierrors.Error
	if !errors.As(wrapped, &got) || got != appError || got.Code() != openapi.PRODUCT_NOT_FOUND {
		t.Fatal("typed application error must be accessible through errors.As")
	}
	if appError.Error() != "Товар не найден" {
		t.Fatal("error text must contain only the public message")
	}
}

func TestStatusCodesMatchCatalog(t *testing.T) {
	expected := map[openapi.ErrorCode]int{
		openapi.PRODUCT_NOT_FOUND:         404,
		openapi.PRODUCT_INACTIVE:          409,
		openapi.ORDER_NOT_FOUND:           404,
		openapi.ORDER_LIMIT_EXCEEDED:      429,
		openapi.ORDER_HAS_ACTIVE:          409,
		openapi.INVALID_STATE_TRANSITION:  409,
		openapi.INSUFFICIENT_STOCK:        409,
		openapi.PROMO_CODE_INVALID:        422,
		openapi.PROMO_CODE_MIN_AMOUNT:     422,
		openapi.ORDER_OWNERSHIP_VIOLATION: 403,
		openapi.VALIDATION_ERROR:          400,
		openapi.INTERNAL_SERVER_ERROR:     500,
		openapi.ROUTE_NOT_FOUND:           404,
		openapi.METHOD_NOT_ALLOWED:        405,
	}
	if len(expected) != len(openapi.AllowedErrorCodeEnumValues) {
		t.Fatal("status mapping must cover the whole generated catalog")
	}
	for _, code := range openapi.AllowedErrorCodeEnumValues {
		t.Run(string(code), func(t *testing.T) {
			want, ok := expected[code]
			if !ok {
				t.Fatalf("missing contract status for %s", code)
			}
			got := apierrors.New(code, "Public message", nil, nil)
			if got.Code() != code || got.StatusCode() != want {
				t.Fatalf("%s: got code %s and status %d, want %d", code, got.Code(), got.StatusCode(), want)
			}
		})
	}
}

func TestGeneratedErrorResponseSerialization(t *testing.T) {
	for _, details := range []map[string]any{
		nil,
		{"fields": map[string]any{"name": []any{"Поле обязательно"}}},
	} {
		appError := apierrors.New(openapi.VALIDATION_ERROR, "Некорректные данные", details, errors.New("private cause"))
		data, err := json.Marshal(appError.Response())
		if err != nil {
			t.Fatal(err)
		}
		var response openapi.ErrorResponse
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(response, appError.Response()) || strings.Contains(string(data), "private") {
			t.Fatalf("unexpected response: %s", data)
		}
		if details == nil && strings.Contains(string(data), `"details"`) {
			t.Fatalf("absent details must be omitted: %s", data)
		}
	}
}
