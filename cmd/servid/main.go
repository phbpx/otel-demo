package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/phbpx/otel-demo/handler"
	"github.com/phbpx/otel-demo/postgres"
	"github.com/riandyrn/otelchi"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {

	log, err := newLog("leads-api")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := run("leads-api", log); err != nil {
		log.Errorw("startup", "err", err)
		os.Exit(1)
	}
}

func run(serverName string, log *zap.SugaredLogger) error {

	// =========================================================================
	// Configuration

	cfg := struct {
		Http struct {
			ReadTimeout     time.Duration `conf:"default:5s"`
			WriteTimeout    time.Duration `conf:"default:10s"`
			IdleTimeout     time.Duration `conf:"default:120s"`
			ShutdownTimeout time.Duration `conf:"default:20s"`
			Host            string        `conf:"default:0.0.0.0:3000"`
		}
		DB struct {
			User         string `conf:"default:leedsvc"`
			Password     string `conf:"default:leedsvc,mask"`
			Host         string `conf:"default:localhost"`
			Name         string `conf:"default:leeds"`
			MaxIdleConns int    `conf:"default:0"`
			MaxOpenConns int    `conf:"default:0"`
			DisableTLS   bool   `conf:"default:true"`
		}
		Jaeger struct {
			ReporterURI string  `conf:"default:http://localhost:14268/api/traces"`
			ServiceName string  `conf:"default:leedsvc-api"`
			Probability float64 `conf:"default:0.5"`
		}
	}{}

	help, err := conf.Parse("LEAD", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(help)
			return nil
		}
		return fmt.Errorf("parsing config: %w", err)
	}

	// =========================================================================
	// Database Support

	// Create connectivity to the database.
	log.Infow("startup", "status", "initializing database support", "host", cfg.DB.Host)

	db, err := postgres.Open(postgres.Config{
		User:         cfg.DB.User,
		Password:     cfg.DB.Password,
		Host:         cfg.DB.Host,
		Name:         cfg.DB.Name,
		MaxIdleConns: cfg.DB.MaxIdleConns,
		MaxOpenConns: cfg.DB.MaxOpenConns,
		DisableTLS:   cfg.DB.DisableTLS,
	})
	if err != nil {
		return fmt.Errorf("connecting to db: %w", err)
	}
	defer func() {
		log.Infow("shutdown", "status", "stopping database support", "host", cfg.DB.Host)
		db.Close()
	}()

	// =========================================================================
	// Update database schema

	log.Infow("startup", "status", "updating database schema", "database", cfg.DB.Name, "host", cfg.DB.Host)

	if err := postgres.Migrate(context.Background(), db); err != nil {
		return fmt.Errorf("updating database schema: %w", err)
	}

	// =========================================================================
	// Start Tracing Support

	log.Infow("startup", "status", "initializing OT/Jaeger tracing support")

	traceProvider, err := startTracing(
		cfg.Jaeger.ServiceName,
		cfg.Jaeger.ReporterURI,
	)
	if err != nil {
		return fmt.Errorf("starting tracing: %w", err)
	}
	defer traceProvider.Shutdown(context.Background())

	// =========================================================================
	// Create router

	log.Infow("startup", "status", "initializing router")

	otelLog := otelzap.New(log.Desugar(), otelzap.WithStackTrace(true)).Sugar()
	leadService := postgres.NewLeadService(db)
	leadHandler := handler.NewLeadHanlder(leadService, otelLog)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(otelchi.Middleware(serverName, otelchi.WithChiRoutes(r)))

	r.Route("/leads", func(r chi.Router) {
		r.Post("/", leadHandler.Create)
		r.Get("/{id}", leadHandler.GetByID)
	})

	// =========================================================================
	// Start API Server

	log.Infow("startup", "status", "initializing http server")

	// Make a channel to listen for an interrupt or terminate signal from the OS.
	// Use a buffered channel because the signal package requires it.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// The HTTP Server
	server := &http.Server{
		Addr:         cfg.Http.Host,
		Handler:      r,
		ReadTimeout:  cfg.Http.ReadTimeout,
		WriteTimeout: cfg.Http.WriteTimeout,
		IdleTimeout:  cfg.Http.IdleTimeout,
		ErrorLog:     zap.NewStdLog(log.Desugar()),
	}

	// Server run context
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	// Listen for syscall signals for process to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig

		// Shutdown signal with grace period of 30 seconds
		shutdownCtx, _ := context.WithTimeout(serverCtx, 30*time.Second)

		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				log.Fatal("graceful shutdown timed out.. forcing exit.")
			}
		}()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal(err)
		}
		serverStopCtx()
	}()

	// Run the server
	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}

	// Wait for server context to be stopped
	<-serverCtx.Done()

	return nil
}

func newLog(serviceName string) (*zap.SugaredLogger, error) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.DisableStacktrace = true
	config.InitialFields = map[string]interface{}{
		"service": serviceName,
	}

	log, err := config.Build()
	if err != nil {
		return nil, err
	}

	return log.Sugar(), nil
}

func startTracing(serviceName, reporterURL string) (*tracesdk.TracerProvider, error) {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(reporterURL)))
	if err != nil {
		return nil, fmt.Errorf("creating new exporter: %w", err)
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp,
			tracesdk.WithMaxExportBatchSize(tracesdk.DefaultMaxExportBatchSize),
			tracesdk.WithBatchTimeout(tracesdk.DefaultScheduleDelay*time.Millisecond),
			tracesdk.WithMaxExportBatchSize(tracesdk.DefaultMaxExportBatchSize),
		),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("exporter", "jaeger"),
		)),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}
