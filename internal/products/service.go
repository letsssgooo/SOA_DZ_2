package products

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/letsssgooo/SOA_DZ_2/internal/apierrors"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

var _ openapi.DefaultAPIServicer = (*Service)(nil)

func (s *Service) CreateProduct(ctx context.Context, product openapi.ProductCreate) (openapi.ImplResponse, error) {
	created, err := s.repository.Create(ctx, product)
	if err != nil {
		return serviceError(err)
	}
	return openapi.Response(http.StatusCreated, created), nil
}

func (s *Service) GetProductById(ctx context.Context, id string) (openapi.ImplResponse, error) {
	product, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return serviceError(err)
	}
	return openapi.Response(http.StatusOK, product), nil
}

func (s *Service) GetProducts(ctx context.Context, page, size int32, status, category string) (openapi.ImplResponse, error) {
	products, err := s.repository.List(ctx, page, size, status, category)
	if err != nil {
		return serviceError(err)
	}
	return openapi.Response(http.StatusOK, products), nil
}

func (s *Service) UpdateProduct(ctx context.Context, id string, product openapi.ProductUpdate) (openapi.ImplResponse, error) {
	updated, err := s.repository.Update(ctx, id, product)
	if err != nil {
		return serviceError(err)
	}
	return openapi.Response(http.StatusOK, updated), nil
}

func (s *Service) ArchiveProduct(ctx context.Context, id string) (openapi.ImplResponse, error) {
	if err := s.repository.Archive(ctx, id); err != nil {
		return serviceError(err)
	}
	return openapi.Response(http.StatusNoContent, nil), nil
}

func serviceError(err error) (openapi.ImplResponse, error) {
	code := openapi.INTERNAL_SERVER_ERROR
	message := "Внутренняя ошибка сервера"
	if errors.Is(err, pgx.ErrNoRows) {
		code = openapi.PRODUCT_NOT_FOUND
		message = "Товар не найден"
	}
	appError := apierrors.New(code, message, nil, err)
	return openapi.Response(appError.StatusCode(), nil), appError
}
