package ingestion

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/metrics"
)

type Handler struct {
	config    *config.Config
	metrics   *metrics.Registry
	logger    zerolog.Logger
	parser    *Parser
	processor Processor
}

type Processor interface {
	AddEntry(entry *LogEntry) error
}

type LogEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	Message       string            `json:"message"`
	Level         string            `json:"level,omitempty"`
	Source        string            `json:"source,omitempty"`
	ContainerName string            `json:"container_name,omitempty"`
	ContainerID   string            `json:"container_id,omitempty"`
	PodName       string            `json:"pod_name,omitempty"`
	Namespace     string            `json:"namespace,omitempty"`
	NodeName      string            `json:"node_name,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type IngestRequest struct {
	Logs []LogEntry `json:"logs,omitempty"`
	
	Log       string            `json:"log,omitempty"`
	Timestamp string            `json:"@timestamp,omitempty"`
	Time      string            `json:"time,omitempty"`
	Stream    string            `json:"stream,omitempty"`
	Tag       string            `json:"tag,omitempty"`
	Source    string            `json:"source,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type IngestResponse struct {
	Status    string `json:"status"`
	Processed int    `json:"processed"`
	Errors    int    `json:"errors,omitempty"`
	Message   string `json:"message,omitempty"`
}

func NewHandler(cfg *config.Config, metrics *metrics.Registry, logger zerolog.Logger) *Handler {
	return &Handler{
		config:  cfg,
		metrics: metrics,
		logger:  logger.With().Str("component", "ingestion").Logger(),
		parser:  NewParser(logger),
	}
}

func (h *Handler) SetProcessor(processor Processor) {
	h.processor = processor
}

func (h *Handler) HandleIngest(c *gin.Context) {
	start := time.Now()
	
	profile := c.Query("profile")
	if profile == "" {
		profile = "auto"
	}
	
	output := c.Query("output")
	source := c.GetHeader("X-Source")
	if source == "" {
		source = "unknown"
	}

	contentLength := c.Request.ContentLength
	if contentLength > h.config.Ingestion.MaxRequestSize {
		h.metrics.IngestionErrorsTotal.WithLabelValues(source, "request_too_large").Inc()
		c.JSON(http.StatusRequestEntityTooLarge, IngestResponse{
			Status:  "error",
			Message: "Request too large",
		})
		return
	}

	var req IngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error().Err(err).Str("source", source).Msg("Failed to parse request")
		h.metrics.IngestionErrorsTotal.WithLabelValues(source, "parse_error").Inc()
		c.JSON(http.StatusBadRequest, IngestResponse{
			Status:  "error",
			Message: fmt.Sprintf("Invalid JSON: %v", err),
		})
		return
	}

	logs, err := h.parser.Parse(&req)
	if err != nil {
		h.logger.Error().Err(err).Str("source", source).Msg("Failed to parse logs")
		h.metrics.IngestionErrorsTotal.WithLabelValues(source, "parse_error").Inc()
		c.JSON(http.StatusBadRequest, IngestResponse{
			Status:  "error",
			Message: fmt.Sprintf("Parse error: %v", err),
		})
		return
	}

	processed := 0
	errors := 0

	for _, logEntry := range logs {
		logEntry.Source = source
		if logEntry.Labels == nil {
			logEntry.Labels = make(map[string]string)
		}
		logEntry.Labels["profile"] = profile
		if output != "" {
			logEntry.Labels["output"] = output
		}

		if h.processor != nil {
			if err := h.processor.AddEntry(&logEntry); err != nil {
				h.logger.Error().Err(err).Msg("Failed to add log to processor")
				errors++
				continue
			}
		}
		processed++
	}

	duration := time.Since(start)
	
	h.metrics.IngestionLogsTotal.WithLabelValues(source, profile).Add(float64(processed))
	h.metrics.IngestionBytesTotal.WithLabelValues(source, profile).Add(float64(contentLength))
	h.metrics.IngestionDuration.WithLabelValues(source, profile).Observe(duration.Seconds())
	
	if errors > 0 {
		h.metrics.IngestionErrorsTotal.WithLabelValues(source, "buffer_error").Add(float64(errors))
	}

	status := "success"
	if errors > 0 {
		status = "partial"
	}

	c.JSON(http.StatusOK, IngestResponse{
		Status:    status,
		Processed: processed,
		Errors:    errors,
	})

	h.logger.Debug().
		Str("source", source).
		Str("profile", profile).
		Int("processed", processed).
		Int("errors", errors).
		Dur("duration", duration).
		Msg("Processed ingest request")
}