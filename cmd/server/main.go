package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/logging"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/router"
	"github.com/letsssgooo/SOA_DZ_2/internal/httpapi/validation"
	"github.com/letsssgooo/SOA_DZ_2/internal/products"
)

const (
	databaseConnectTimeout = 5 * time.Second
	readHeaderTimeout      = 5 * time.Second
	serverShutdownTimeout  = 5 * time.Second
)

func main() {
	slog.SetDefault(logging.NewLogger(os.Stdout))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("Application failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	validator, err := validation.LoadRequestValidator(ctx, "api/openapi.yaml")
	if err != nil {
		return err
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	connectCtx, cancel := context.WithTimeout(ctx, databaseConnectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectCtx, config)
	if err != nil {
		return errors.New("cannot create PostgreSQL connection pool")
	}
	defer pool.Close()
	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("PostgreSQL connection check failed: %w", err)
	}
	cancel()
	slog.Info("PostgreSQL connection established")

	repository := products.NewRepository(pool)
	service := products.NewService(repository)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           router.NewRouter(service, validator),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	slog.Info("HTTP server listening", "addr", server.Addr)
	defer server.Close()
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server failed: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP server shutdown failed: %w", err)
		}
		return nil
	}
}
