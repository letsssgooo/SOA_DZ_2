package products

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/letsssgooo/SOA_DZ_2/internal/gen/openapi"
)

const productColumns = `id, name, description, price::real, stock, category,
                        status, created_at, updated_at`

const productFilters = `
	WHERE ($1::product_status IS NULL OR status = $1)
	  AND ($2::text IS NULL OR category = $2)`

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByID(ctx context.Context, id string) (openapi.ProductResponse, error) {
	const query = `SELECT ` + productColumns + ` FROM products WHERE id = $1`
	row := r.pool.QueryRow(ctx, query, id)
	return scanProduct(row)
}

func (r *Repository) Create(ctx context.Context, product openapi.ProductCreate) (openapi.ProductResponse, error) {
	const query = `
		INSERT INTO products (name, description, price, stock, category, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + productColumns
	row := r.pool.QueryRow(ctx, query,
		product.Name, product.Description, product.Price, product.Stock, product.Category, product.Status,
	)
	return scanProduct(row)
}

func (r *Repository) List(ctx context.Context, page, size int32, status, category string) (openapi.GetProducts200Response, error) {
	result := openapi.GetProducts200Response{
		Items: make([]openapi.ProductResponse, 0),
		Page:  page,
		Size:  size,
	}
	var statusFilter, categoryFilter *string
	if status != "" {
		statusFilter = &status
	}
	if category != "" {
		categoryFilter = &category
	}
	const countQuery = `SELECT COUNT(*) FROM products` + productFilters
	if err := r.pool.QueryRow(ctx, countQuery, statusFilter, categoryFilter).Scan(&result.TotalElements); err != nil {
		return result, err
	}
	const query = `SELECT ` + productColumns + ` FROM products` + productFilters + ` ORDER BY id LIMIT $3 OFFSET $4`
	offset := int64(page) * int64(size)
	rows, err := r.pool.Query(ctx, query, statusFilter, categoryFilter, size, offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, product)
	}
	return result, rows.Err()
}

func (r *Repository) Update(ctx context.Context, id string, product openapi.ProductUpdate) (openapi.ProductResponse, error) {
	const query = `
		UPDATE products
		SET name = $2, description = $3, price = $4, stock = $5, category = $6, status = $7
		WHERE id = $1
		RETURNING ` + productColumns
	row := r.pool.QueryRow(ctx, query, id,
		product.Name, product.Description, product.Price, product.Stock, product.Category, product.Status,
	)
	return scanProduct(row)
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	const query = `UPDATE products SET status = 'ARCHIVED' WHERE id = $1`
	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func scanProduct(row pgx.Row) (openapi.ProductResponse, error) {
	var product openapi.ProductResponse
	err := row.Scan(
		&product.Id, &product.Name, &product.Description, &product.Price,
		&product.Stock, &product.Category, &product.Status,
		&product.CreatedAt, &product.UpdatedAt,
	)
	return product, err
}
