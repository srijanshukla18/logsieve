package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
	"github.com/logsieve/logsieve/pkg/processor"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Setup logging
	logger := setupLogging()
	logger.Info().
		Str("version", Version).
		Str("commit", Commit).
		Str("build_time", BuildTime).
		Msg("Starting LogSieve server")

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize metrics
	metricsRegistry := metrics.NewRegistry()
	metricsRegistry.SetBuildInfo(Version, Commit, BuildTime)
	metricsRegistry.SetStartTime(float64(time.Now().Unix()))

	// Start uptime updater
	go updateUptime(metricsRegistry, logger)

	// Initialize processor
	proc, err := processor.NewProcessor(cfg, metricsRegistry, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create processor")
	}

	// Start processor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := proc.Start(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start processor")
	}
	defer proc.Stop()

	// Setup HTTP server
	router := setupRouter(cfg, metricsRegistry, proc, logger)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start metrics server
	metricsServer := startMetricsServer(cfg, logger)
	defer func() {
		if err := metricsServer.Shutdown(context.Background()); err != nil {
			logger.Error().Err(err).Msg("Error shutting down metrics server")
		}
	}()

	// Start HTTP server in goroutine
	go func() {
		logger.Info().
			Str("address", server.Addr).
			Msg("Starting HTTP server")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Server forced to shutdown")
	}

	logger.Info().Msg("Server exited")
}

func setupLogging() zerolog.Logger {
	// Check environment for log level
	logLevel := os.Getenv("LOGSIEVE_LOGGING_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)

	// Use console output if not in production
	if os.Getenv("LOGSIEVE_LOGGING_FORMAT") == "console" {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	}

	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}

func loadConfig() (*config.Config, error) {
	// Set config file path
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/logsieve/")
	viper.AddConfigPath("$HOME/.logsieve")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	// Enable environment variable overrides
	viper.SetEnvPrefix("LOGSIEVE")
	viper.AutomaticEnv()

	// Set defaults
	cfg := config.DefaultConfig()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; use defaults
			fmt.Println("No config file found, using defaults")
		} else {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	} else {
		// Unmarshal config
		if err := viper.Unmarshal(cfg); err != nil {
			return nil, fmt.Errorf("error unmarshaling config: %w", err)
		}
	}

	return cfg, nil
}

func setupRouter(cfg *config.Config, metricsRegistry *metrics.Registry, proc *processor.Processor, logger zerolog.Logger) *gin.Engine {
	// Set gin mode
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Add middleware
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware(logger))
	router.Use(metricsMiddleware(metricsRegistry))

	// Setup ingestion handler
	handler := ingestion.NewHandler(cfg, metricsRegistry, logger)
	handler.SetProcessor(proc)

	// Routes
	router.GET("/health", healthHandler)
	router.GET("/ready", readyHandler(proc))
	router.GET("/stats", statsHandler(proc))
	router.POST("/ingest", handler.HandleIngest)

	return router
}

func startMetricsServer(cfg *config.Config, logger zerolog.Logger) *http.Server {
	if !cfg.Metrics.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.Metrics.Path, promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Metrics.Port),
		Handler: mux,
	}

	go func() {
		logger.Info().
			Int("port", cfg.Metrics.Port).
			Str("path", cfg.Metrics.Path).
			Msg("Starting metrics server")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("Metrics server error")
		}
	}()

	return server
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().UTC(),
	})
}

func readyHandler(proc *processor.Processor) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats := proc.GetStats()

		if !stats.Running {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": "processor not running",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"time":   time.Now().UTC(),
		})
	}
}

func statsHandler(proc *processor.Processor) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats := proc.GetStats()
		c.JSON(http.StatusOK, stats)
	}
}

func loggingMiddleware(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info().
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", statusCode).
			Dur("latency", latency).
			Str("client_ip", c.ClientIP()).
			Msg("HTTP request")
	}
}

func metricsMiddleware(metricsRegistry *metrics.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		c.Next()

		duration := time.Since(start)
		status := fmt.Sprintf("%d", c.Writer.Status())

		metricsRegistry.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		metricsRegistry.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration.Seconds())

		if c.Request.ContentLength > 0 {
			metricsRegistry.HTTPRequestSize.WithLabelValues(c.Request.Method, path).Observe(float64(c.Request.ContentLength))
		}

		metricsRegistry.HTTPResponseSize.WithLabelValues(c.Request.Method, path).Observe(float64(c.Writer.Size()))
	}
}

func updateUptime(metricsRegistry *metrics.Registry, logger zerolog.Logger) {
	startTime := time.Now()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		uptime := time.Since(startTime).Seconds()
		metricsRegistry.UpdateUptime(uptime)
	}
}
