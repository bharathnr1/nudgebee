package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"nudgebee/tickets-server/common"
	"nudgebee/tickets-server/routes"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	slogformatter "github.com/samber/slog-formatter"
	sloggin "github.com/samber/slog-gin"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var errorFormatter = slogformatter.ErrorFormatter("error")
var logger = slog.New(
	slogformatter.NewFormatterHandler(errorFormatter)(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	),
)

func main() {
	slog.SetDefault(logger)
	tp, mp, err := initOtel()
	if err != nil {
		slog.Error(err.Error())
		return
	}

	defer func() {
		tpSdk, ok := tp.(*sdktrace.TracerProvider)
		if ok {
			if err := tpSdk.Shutdown(context.Background()); err != nil {
				slog.Error(fmt.Sprintf("Error shutting down tracer provider: %v", err))
			}
		}
		mpSdk, ok := mp.(*sdkmetric.MeterProvider)
		if ok {
			if err := mpSdk.Shutdown(context.Background()); err != nil {
				slog.Error(fmt.Sprintf("Error shutting down meter provider: %v", err))
			}
		}
	}()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	// ticket-server has no global inbound service-token middleware (it relies on
	// network isolation), so there is no auth gate to register pprof behind.
	// pprof heap/goroutine dumps can leak in-flight request state, so it must
	// not be served unauthenticated by default — make it explicit opt-in via
	// PPROF_ENABLED=true for diagnostics.
	if strings.EqualFold(os.Getenv("PPROF_ENABLED"), "true") {
		pprof.Register(router)
		slog.Warn("pprof debug endpoints enabled at /debug/pprof (PPROF_ENABLED=true) — ensure this service is not externally reachable")
	}
	router.Use(gin.Recovery())
	router.Use(sloggin.NewWithFilters(logger, sloggin.IgnorePath("/health")))

	routes.InitializeRoutes(router)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", common.Config.Port),
		Handler: router,
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		slog.Info("Got SIGTERM, shutting down")
		slog.Info("Connections closed, shutting down server")
		err := srv.Shutdown(context.Background())
		if err != nil {
			slog.Error("Server shutdown failed:", "error", err)
		}
		os.Exit(1)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Server listen failed:", "error", err)
	}
}
