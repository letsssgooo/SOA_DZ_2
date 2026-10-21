package validation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
	httperrors "github.com/letsssgooo/SOA_DZ_2/internal/httpapi/errors"
)

type RequestValidator struct {
	router  routers.Router
	options openapi3filter.Options
}

func LoadRequestValidator(ctx context.Context, path string) (*RequestValidator, error) {
	loader := openapi3.NewLoader()
	loader.Context = ctx
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = openapi3.ReadFromFile
	document, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI specification: %w", err)
	}
	return newRequestValidator(document)
}

func newRequestValidator(document *openapi3.T) (*RequestValidator, error) {
	router, err := legacy.NewRouter(document)
	if err != nil {
		return nil, fmt.Errorf("validate OpenAPI specification and build routes: %w", err)
	}
	return &RequestValidator{
		router: router,
		options: openapi3filter.Options{
			MultiError:          true,
			SkipSettingDefaults: true,
			SchemaValidationOptions: []openapi3.SchemaValidationOption{
				openapi3.EnableFormatValidation(),
				openapi3.DisableSchemaErrorDetails(),
				openapi3.WithStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(
					`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
				)),
			},
		},
	}, nil
}

func (v *RequestValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, pathParams, err := v.router.FindRoute(r)
		if err != nil {
			var routeError *routers.RouteError
			if errors.As(err, &routeError) {
				next.ServeHTTP(w, r)
			} else {
				httperrors.HandleError(w, r, err, nil)
			}
			return
		}
		input := &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: pathParams,
			Route:      route,
			Options:    &v.options,
		}
		if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
			fields := make(map[string][]string)
			if !collectValidationFields(err, "body", fields) {
				httperrors.HandleError(w, r, err, nil)
				return
			}
			httperrors.WriteError(w, r, apierrors.New(openapi.VALIDATION_ERROR, "Ошибка валидации запроса", map[string]any{"fields": fields}, nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func collectValidationFields(err error, field string, fields map[string][]string) bool {
	switch e := err.(type) {
	case openapi3.MultiError:
		for _, nested := range e {
			if !collectValidationFields(nested, field, fields) {
				return false
			}
		}
		return len(e) > 0
	case *openapi3filter.RequestError:
		if e.Parameter != nil {
			field = e.Parameter.Name
		}
		if e.Err == nil {
			addValidationField(fields, field, "Недопустимый формат запроса")
			return true
		}
		if errors.Is(e.Err, openapi3filter.ErrInvalidRequired) {
			addValidationField(fields, field, "Поле обязательно")
			return true
		}
		if errors.Is(e.Err, openapi3filter.ErrInvalidEmptyValue) {
			addValidationField(fields, field, "Пустое значение недопустимо")
			return true
		}
		return collectValidationFields(e.Err, field, fields)
	case *openapi3.SchemaError:
		if path := validationFieldPath(e.JSONPointer()); path != "" {
			field = path
		}
		message := "Недопустимое значение"
		switch e.SchemaField {
		case "required":
			message = "Поле обязательно"
		case "type":
			message = "Недопустимый тип или null"
		case "minLength", "maxLength":
			message = "Недопустимая длина строки"
		case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum":
			message = "Значение вне допустимого диапазона"
		case "enum":
			message = "Значение отсутствует в списке допустимых"
		case "pattern", "format":
			message = "Недопустимый формат значения"
		case "minItems", "maxItems":
			message = "Недопустимое количество элементов"
		case "properties", "additionalProperties":
			message = "Неизвестное поле"
		}
		addValidationField(fields, field, message)
		return true
	case *openapi3filter.ParseError:
		if e.Kind != openapi3filter.KindInvalidFormat {
			return false
		}
		addValidationField(fields, field, "Некорректный формат запроса")
		return true
	default:
		return false
	}
}

func addValidationField(fields map[string][]string, field, message string) {
	if !slices.Contains(fields[field], message) {
		fields[field] = append(fields[field], message)
	}
}

func validationFieldPath(parts []string) string {
	var path strings.Builder
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err == nil {
			path.WriteString("[" + part + "]")
		} else {
			if path.Len() > 0 {
				path.WriteByte('.')
			}
			path.WriteString(part)
		}
	}
	return path.String()
}
