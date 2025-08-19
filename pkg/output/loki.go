package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
)

type LokiAdapter struct {
	config     config.OutputConfig
	logger     zerolog.Logger
	httpClient *http.Client
	pushURL    string
}

// LokiPushRequest represents the official Loki Push API request format
type LokiPushRequest struct {
	Streams []LokiStream `json:"streams"`
}

// LokiStream represents a log stream in the official Loki format
type LokiStream struct {
	Stream map[string]string `json:"stream"`           // Labels as key-value pairs
	Values [][]interface{}   `json:"values"`           // [timestamp, log_line, structured_metadata?]
}

// LokiEntry represents a single log entry with optional structured metadata
type LokiEntry struct {
	Timestamp         string                 `json:"ts"`
	Line              string                 `json:"line"`
	StructuredMetadata map[string]interface{} `json:"structuredMetadata,omitempty"`
}

func NewLokiAdapter(config config.OutputConfig, logger zerolog.Logger) (*LokiAdapter, error) {
	pushURL := strings.TrimSuffix(config.URL, "/") + "/loki/api/v1/push"

	return &LokiAdapter{
		config:  config,
		logger:  logger.With().Str("adapter", "loki").Logger(),
		pushURL: pushURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

func (l *LokiAdapter) Send(entries []*ingestion.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	streams := l.groupByLabels(entries)
	
	request := LokiPushRequest{
		Streams: streams,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal Loki request: %w", err)
	}

	req, err := http.NewRequest("POST", l.pushURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	
	// Add authentication headers if configured
	for key, value := range l.config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Loki returns 204 No Content on successful ingestion
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		// Read error response body for better error reporting
		var errorMsg string
		if body, readErr := io.ReadAll(resp.Body); readErr == nil {
			errorMsg = string(body)
		}
		return fmt.Errorf("Loki returned status %d: %s", resp.StatusCode, errorMsg)
	}

	l.logger.Debug().
		Int("entries", len(entries)).
		Int("streams", len(streams)).
		Str("url", l.pushURL).
		Msg("Sent entries to Loki")

	return nil
}

func (l *LokiAdapter) groupByLabels(entries []*ingestion.LogEntry) []LokiStream {
	streamMap := make(map[string]*LokiStream)

	for _, entry := range entries {
		labels := l.extractLabels(entry)
		streamKey := l.createStreamKey(labels)

		stream, exists := streamMap[streamKey]
		if !exists {
			stream = &LokiStream{
				Stream: labels,
				Values: [][]string{},
			}
			streamMap[streamKey] = stream
		}

		timestamp := strconv.FormatInt(entry.Timestamp.UnixNano(), 10)
		
		// Create log entry with optional structured metadata
		logValue := []interface{}{timestamp, entry.Message}
		
		// Add structured metadata if present (Loki v3+ feature)
		if structuredMetadata := l.extractStructuredMetadata(entry); len(structuredMetadata) > 0 {
			logValue = append(logValue, structuredMetadata)
		}
		
		stream.Values = append(stream.Values, logValue)
	}

	streams := make([]LokiStream, 0, len(streamMap))
	for _, stream := range streamMap {
		streams = append(streams, *stream)
	}

	return streams
}

func (l *LokiAdapter) extractLabels(entry *ingestion.LogEntry) map[string]string {
	labels := make(map[string]string)

	// Core labels that should be indexed (low cardinality)
	if entry.Level != "" {
		labels["level"] = entry.Level
	}
	
	if entry.Source != "" {
		labels["source"] = entry.Source
	}
	
	if entry.ContainerName != "" {
		labels["container"] = entry.ContainerName
	}
	
	if entry.PodName != "" {
		labels["pod"] = entry.PodName
	}
	
	if entry.Namespace != "" {
		labels["namespace"] = entry.Namespace
	}

	// Add custom labels, but be careful about cardinality
	for key, value := range entry.Labels {
		if value != "" && !strings.HasPrefix(key, "context_") && !l.isHighCardinalityLabel(key) {
			labels[key] = value
		}
	}

	// Ensure at least one label exists (Loki requirement)
	if len(labels) == 0 {
		labels["job"] = "logsieve"
	}

	return labels
}

// extractStructuredMetadata extracts high-cardinality data as structured metadata (Loki v3+)
func (l *LokiAdapter) extractStructuredMetadata(entry *ingestion.LogEntry) map[string]interface{} {
	metadata := make(map[string]interface{})
	
	// Add high-cardinality labels as structured metadata
	for key, value := range entry.Labels {
		if value != "" && l.isHighCardinalityLabel(key) {
			metadata[key] = value
		}
	}
	
	// Add context information as structured metadata
	for key, value := range entry.Labels {
		if strings.HasPrefix(key, "context_") {
			metadata[key] = value
		}
	}
	
	return metadata
}

// isHighCardinalityLabel determines if a label should be stored as structured metadata
// instead of an index label to avoid cardinality issues
func (l *LokiAdapter) isHighCardinalityLabel(key string) bool {
	highCardinalityLabels := map[string]bool{
		"request_id":    true,
		"trace_id":      true,
		"user_id":       true,
		"session_id":    true,
		"transaction_id": true,
		"correlation_id": true,
		"ip_address":    true,
		"user_agent":    true,
		"url":           true,
		"path":          true,
		"query":         true,
		"timestamp":     true,
		"duration":      true,
		"response_time": true,
	}
	
	return highCardinalityLabels[key]
}

func (l *LokiAdapter) createStreamKey(labels map[string]string) string {
	// Create a consistent, sorted key for stream grouping
	keys := make([]string, 0, len(labels))
	for key, value := range labels {
		keys = append(keys, fmt.Sprintf("%s=%q", key, value)) // Use quoted values for consistency
	}
	
	// Sort for consistent ordering
	sort.Strings(keys)
	
	return strings.Join(keys, ",")
}

func (l *LokiAdapter) Name() string {
	return l.config.Name
}

func (l *LokiAdapter) Close() error {
	return nil
}