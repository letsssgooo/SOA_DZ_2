package errors

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

const internalErrorMessage = "Внутренняя ошибка сервера"

func HandleError(w http.ResponseWriter, r *http.Request, err error, _ *openapi.ImplResponse) {
	var appError *apierrors.Error
	if errors.As(err, &appError) && appError != nil {
		if appError.StatusCode() >= http.StatusInternalServerError {
			logInternalError(r, err)
			WriteError(w, r, internalError())
			return
		}
		WriteError(w, r, appError)
		return
	}

	var required *openapi.RequiredError
	if errors.As(err, &required) && required != nil {
		WriteError(w, r, validationError(required.Field, "Поле обязательно"))
		return
	}
	var parsing *openapi.ParsingError
	if errors.As(err, &parsing) && parsing != nil {
		if parsing.Param != "" {
			WriteError(w, r, validationError(parsing.Param, "Недопустимое значение"))
		} else if errors.Is(parsing.Err, io.EOF) {
			WriteError(w, r, validationError("body", "Тело запроса обязательно"))
		} else {
			WriteError(w, r, validationError("body", "Некорректный формат запроса"))
		}
		return
	}
	var syntax *json.SyntaxError
	var valueType *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &valueType) {
		field := "body"
		if valueType != nil && valueType.Field != "" {
			field = valueType.Field
		}
		WriteError(w, r, validationError(field, "Некорректный JSON"))
		return
	}

	logInternalError(r, err)
	WriteError(w, r, internalError())
}

func validationError(field, message string) *apierrors.Error {
	return apierrors.New(openapi.VALIDATION_ERROR, "Ошибка валидации запроса", map[string]any{
		"fields": map[string][]string{jsonFieldName(field): {message}},
	}, nil)
}

func jsonFieldName(field string) string {
	for _, model := range []reflect.Type{
		reflect.TypeOf(openapi.ProductCreate{}),
		reflect.TypeOf(openapi.ProductUpdate{}),
	} {
		if f, ok := model.FieldByName(field); ok {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name != "" && name != "-" {
				return name
			}
		}
	}
	return field
}

func internalError() *apierrors.Error {
	return apierrors.New(openapi.INTERNAL_SERVER_ERROR, internalErrorMessage, nil, nil)
}

// WriteError sends an application error using the generated response schema.
func WriteError(w http.ResponseWriter, r *http.Request, err *apierrors.Error) {
	body, marshalErr := json.Marshal(err.Response())
	if marshalErr != nil {
		logInternalError(r, marshalErr)
		err = internalError()
		body, _ = json.Marshal(err.Response())
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode())
	if _, writeErr := w.Write(append(body, '\n')); writeErr != nil {
		logInternalError(r, writeErr)
	}
}

func logInternalError(r *http.Request, err error) {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	attributes := []any{"error", err}
	var appError *apierrors.Error
	if errors.As(err, &appError) && appError != nil && appError.Unwrap() != nil {
		attributes = append(attributes, "cause", appError.Unwrap())
	}
	slog.ErrorContext(ctx, "HTTP request failed", attributes...)
}

// RecoverPanics handles panics without replacing a response already sent.
func RecoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				slog.ErrorContext(r.Context(), "HTTP request panicked", "panic", recovered, "stack", string(debug.Stack()))
				if writer.Status() == 0 {
					id := writer.Header().Get("X-Request-Id")
					clear(writer.Header())
					if id != "" {
						writer.Header().Set("X-Request-Id", id)
					}
					WriteError(writer, r, internalError())
				}
			}
		}()
		next.ServeHTTP(writer, r)
	})
}
