package router

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
	httperrors "github.com/letsssgooo/SOA_DZ_2/internal/httpapi/errors"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/logging"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/validation"
)

func NewRouter(service openapi.DefaultAPIServicer, validator *validation.RequestValidator) http.Handler {
	if validator == nil {
		panic("request validator is required")
	}
	controller := openapi.NewDefaultAPIController(service, openapi.WithDefaultAPIErrorHandler(httperrors.HandleError))
	router := chi.NewRouter()
	var methods []string
	for _, route := range controller.OrderedRoutes() {
		router.Method(route.Method, route.Pattern, route.HandlerFunc)
		methods = append(methods, route.Method)
	}
	slices.Sort(methods)
	methods = slices.Compact(methods)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httperrors.WriteError(w, r, apierrors.New(openapi.ROUTE_NOT_FOUND, "Маршрут не найден", nil, nil))
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		var allowed []string
		for _, method := range methods {
			if router.Match(chi.NewRouteContext(), method, r.URL.Path) {
				allowed = append(allowed, method)
			}
		}
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
		}
		httperrors.WriteError(w, r, apierrors.New(openapi.METHOD_NOT_ALLOWED, "Метод не поддерживается", nil, nil))
	})
	return logging.Middleware(slog.Default(), httperrors.RecoverPanics(validator.Middleware(router)))
}
