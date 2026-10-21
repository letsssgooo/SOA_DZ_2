package products

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

func TestServiceErrorPreservesCause(t *testing.T) {
	for _, tt := range []struct {
		cause  error
		status int
		code   openapi.ErrorCode
	}{
		{fmt.Errorf("lookup: %w", pgx.ErrNoRows), http.StatusNotFound, openapi.PRODUCT_NOT_FOUND},
		{&pgconn.PgError{Code: "22P02", Message: "private invalid input error"}, http.StatusInternalServerError, openapi.INTERNAL_SERVER_ERROR},
	} {
		result, err := serviceError(tt.cause)
		var appError *apierrors.Error
		if !errors.As(err, &appError) || appError.Code() != tt.code || appError.StatusCode() != tt.status || result.Code != tt.status {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if !errors.Is(err, tt.cause) {
			t.Fatalf("lost repository cause: %v", err)
		}
	}
}
