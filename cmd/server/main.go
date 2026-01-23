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
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

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
	var cfgFile string
	var logLevel string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "LogSieve HTTP server",
		Long:  `LogSieve HTTP server for log ingestion, deduplication, and routing`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cfgFile, logLevel)
		},
	}

	cmd.Flags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	cmd.Flags().StringVarP(&logLevel, "log-level", "l", "", "log level (trace, debug, info, warn, error)")
	cmd.Flags().Bool("version", false, "show version information")

	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
			fmt.Printf("LogSieve Server\n")
			fmt.Printf("Version: %s\n", Version)
			fmt.Printf("Commit: %s\n", Commit)
			fmt.Printf("Build Time: %s\n", BuildTime)
			os.Exit(0)
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runServer(cfgFile, logLevel string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	logger := setupLogging(cfg.Logging)
	log.Logger = logger

	log.Info().
		Str("version", Version).
		Str("commit", Commit).
		Str("build_time", BuildTime).
		Msg("Starting LogSieve server")

	metricsRegistry := metrics.NewRegistry()
	
	logProcessor, err := processor.NewProcessor(cfg, metricsRegistry, logger)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create processor")
	}
	
	ingestionHandler := ingestion.NewHandler(cfg, metricsRegistry, logger)
	ingestionHandler.SetProcessor(logProcessor)

	router := setupRouter(cfg, ingestionHandler, logProcessor, metricsRegistry)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	var metricsServer *http.Server
	if cfg.Metrics.Enabled {
		metricsRouter := gin.New()
		metricsRouter.Use(gin.Recovery())
		metricsRouter.GET(cfg.Metrics.Path, gin.WrapH(promhttp.HandlerFor(metricsRegistry.Registry, promhttp.HandlerOpts{})))
		metricsRouter.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "healthy"})
		})

		metricsServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Metrics.Port),
			Handler: metricsRouter,
		}
	}

	errChan := make(chan error, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := logProcessor.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to start processor")
	}

	go func() {
		log.Info().
			Str("address", server.Addr).
			Msg("Starting HTTP server")
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	if metricsServer != nil {
		go func() {
			log.Info().
				Str("address", metricsServer.Addr).
				Str("path", cfg.Metrics.Path).
				Msg("Starting metrics server")
			
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("metrics server error: %w", err)
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	}

	log.Info().Msg("Shutting down servers...")

	cancel()

	if err := logProcessor.Stop(); err != nil {
		log.Error().Err(err).Msg("Processor shutdown error")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Metrics server shutdown error")
		}
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
		return err
	}

	log.Info().Msg("Server shutdown completed")
	return nil
}

func setupLogging(cfg config.LoggingConfig) zerolog.Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)

	var logger zerolog.Logger
	if cfg.Structured {
		if cfg.Output == "stdout" {
			logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
		} else {
			logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
		}
	} else {
		logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	}

	return logger
}

func setupRouter(cfg *config.Config, handler *ingestion.Handler, processor *processor.Processor, metricsRegistry *metrics.Registry) *gin.Engine {
	if cfg.Logging.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	
	router.Use(gin.Recovery())
	router.Use(requestLogger())
	router.Use(metricsMiddleware(metricsRegistry))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"version":   Version,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
		})
	})

	router.GET("/stats", func(c *gin.Context) {
		stats := processor.GetStats()
		c.JSON(http.StatusOK, stats)
	})

	router.POST("/ingest", handler.HandleIngest)
	
	return router
}

func requestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		log.Info().
			Str("method", param.Method).
			Str("path", param.Path).
			Int("status", param.StatusCode).
			Dur("latency", param.Latency).
			Str("client_ip", param.ClientIP).
			Str("user_agent", param.Request.UserAgent()).
			Msg("HTTP request")
		return ""
	})
}

func metricsMiddleware(registry *metrics.Registry) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        c.Next()
        
        duration := time.Since(start)
        // Match metrics label schema: method,status
        registry.HTTPRequests.WithLabelValues(
            c.Request.Method,
            fmt.Sprintf("%d", c.Writer.Status()),
        ).Inc()
        
        registry.HTTPDuration.WithLabelValues(
            c.Request.Method,
            c.FullPath(),
        ).Observe(duration.Seconds())
    }
}
