package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
	httperrors "github.com/letsssgooo/SOA_DZ_2/internal/httpapi/errors"
)

const RequestIDHeader = "X-Request-Id"

func NewLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && attr.Key == slog.TimeKey {
				attr.Key = "timestamp"
			}
			return attr
		},
	}))
}

func Middleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id := uuid.NewString()
		writer := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		writer.Header().Set(RequestIDHeader, id)
		var body any
		mutating := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete
		defer func() {
			status := writer.Status()
			if status == 0 {
				status = http.StatusOK
			}
			attributes := []any{
				"request_id", id,
				"method", r.Method,
				"endpoint", r.URL.Path,
				"status_code", status,
				"duration_ms", float64(time.Since(started)) / float64(time.Millisecond),
				"user_id", nil,
			}
			if mutating {
				attributes = append(attributes, "request_body", body)
			}
			logger.InfoContext(r.Context(), "API request", attributes...)
		}()
		if mutating {
			var err error
			body, err = captureBody(r)
			if err != nil {
				httperrors.WriteError(writer, r, apierrors.New(openapi.VALIDATION_ERROR, "Не удалось прочитать тело запроса", map[string]any{
					"fields": map[string][]string{"body": {"Ошибка чтения тела запроса"}},
				}, err))
				return
			}
		}
		next.ServeHTTP(writer, r)
	})
}

func captureBody(r *http.Request) (any, error) {
	if r.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	if len(data) == 0 {
		return nil, nil
	}
	var body any
	if err := json.Unmarshal(data, &body); err != nil {
		return "[body omitted: invalid JSON]", nil
	}
	if object, ok := body.(map[string]any); ok {
		for key := range object {
			if strings.EqualFold(key, "password") {
				object[key] = "***"
			}
		}
	}
	return body, nil
}
