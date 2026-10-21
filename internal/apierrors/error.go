package apierrors

import (
	"net/http"

	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

var statusCodes = map[openapi.ErrorCode]int{
	openapi.PRODUCT_NOT_FOUND:         http.StatusNotFound,
	openapi.PRODUCT_INACTIVE:          http.StatusConflict,
	openapi.ORDER_NOT_FOUND:           http.StatusNotFound,
	openapi.ORDER_LIMIT_EXCEEDED:      http.StatusTooManyRequests,
	openapi.ORDER_HAS_ACTIVE:          http.StatusConflict,
	openapi.INVALID_STATE_TRANSITION:  http.StatusConflict,
	openapi.INSUFFICIENT_STOCK:        http.StatusConflict,
	openapi.PROMO_CODE_INVALID:        http.StatusUnprocessableEntity,
	openapi.PROMO_CODE_MIN_AMOUNT:     http.StatusUnprocessableEntity,
	openapi.ORDER_OWNERSHIP_VIOLATION: http.StatusForbidden,
	openapi.VALIDATION_ERROR:          http.StatusBadRequest,
	openapi.INTERNAL_SERVER_ERROR:     http.StatusInternalServerError,
	openapi.ROUTE_NOT_FOUND:           http.StatusNotFound,
	openapi.METHOD_NOT_ALLOWED:        http.StatusMethodNotAllowed,
}

type Error struct {
	code    openapi.ErrorCode
	message string
	details map[string]any
	cause   error
}

func New(code openapi.ErrorCode, message string, details map[string]any, cause error) *Error {
	if _, ok := statusCodes[code]; !ok {
		code = openapi.INTERNAL_SERVER_ERROR
		message = "Внутренняя ошибка сервера"
		details = nil
	}
	return &Error{code: code, message: message, details: details, cause: cause}
}

func (e *Error) Error() string {
	return e.message
}

func (e *Error) Code() openapi.ErrorCode {
	return e.code
}

func (e *Error) StatusCode() int {
	if status, ok := statusCodes[e.code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

func (e *Error) Unwrap() error {
	return e.cause
}

func (e *Error) Response() openapi.ErrorResponse {
	return openapi.ErrorResponse{
		ErrorCode: e.code,
		Message:   e.message,
		Details:   e.details,
	}
}
