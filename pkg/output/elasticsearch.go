package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
)

type ElasticsearchAdapter struct {
	config     config.OutputConfig
	logger     zerolog.Logger
	httpClient *http.Client
	bulkURL    string
	indexName  string
}

// ESDocument represents a log document following Elastic Common Schema (ECS) conventions
type ESDocument struct {
	Timestamp     time.Time         `json:"@timestamp"`
	Message       string            `json:"message"`
	Level         string            `json:"log.level,omitempty"`        // ECS compliant
	Source        string            `json:"log.origin.file.name,omitempty"` // ECS compliant
	ContainerName string            `json:"container.name,omitempty"`   // ECS compliant
	ContainerID   string            `json:"container.id,omitempty"`     // ECS compliant
	PodName       string            `json:"kubernetes.pod.name,omitempty"` // ECS compliant
	Namespace     string            `json:"kubernetes.namespace,omitempty"` // ECS compliant
	NodeName      string            `json:"kubernetes.node.name,omitempty"` // ECS compliant
	Labels        map[string]string `json:"labels,omitempty"`
	// Additional ECS fields
	Host          ESHost            `json:"host,omitempty"`
	Service       ESService         `json:"service,omitempty"`
}

// ESHost represents host information in ECS format
type ESHost struct {
	Name string `json:"name,omitempty"`
	IP   string `json:"ip,omitempty"`
}

// ESService represents service information in ECS format
type ESService struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// ESBulkResponse represents the response from Elasticsearch bulk API
type ESBulkResponse struct {
	Took   int                      `json:"took"`
	Errors bool                     `json:"errors"`
	Items  []map[string]interface{} `json:"items"`
}

func NewElasticsearchAdapter(config config.OutputConfig, logger zerolog.Logger) (*ElasticsearchAdapter, error) {
	bulkURL := strings.TrimSuffix(config.URL, "/") + "/_bulk"
	
	indexName := "logs"
	if config.Config != nil {
		if idx, ok := config.Config["index"].(string); ok && idx != "" {
			indexName = idx
		}
	}

	return &ElasticsearchAdapter{
		config:    config,
		logger:    logger.With().Str("adapter", "elasticsearch").Logger(),
		bulkURL:   bulkURL,
		indexName: indexName,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

func (e *ElasticsearchAdapter) Send(entries []*ingestion.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	var buf bytes.Buffer

	for _, entry := range entries {
		indexName := e.getIndexName(entry)
		
		indexAction := map[string]interface{}{
			"index": map[string]string{
				"_index": indexName,
			},
		}

		indexLine, err := json.Marshal(indexAction)
		if err != nil {
			return fmt.Errorf("failed to marshal index action: %w", err)
		}

		doc := e.convertToESDocument(entry)
		docLine, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal document: %w", err)
		}

		buf.Write(indexLine)
		buf.WriteByte('\n')
		buf.Write(docLine)
		buf.WriteByte('\n')
	}

	req, err := http.NewRequest("POST", e.bulkURL, &buf)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Elasticsearch bulk API accepts both content types
	req.Header.Set("Content-Type", "application/x-ndjson")
	
	// Add authentication and custom headers
	for key, value := range e.config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for detailed error reporting
	var bulkResponse ESBulkResponse
	if err := json.NewDecoder(resp.Body).Decode(&bulkResponse); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Elasticsearch returned status %d", resp.StatusCode)
	}

	// Check for individual operation errors
	if bulkResponse.Errors {
		errorDetails := e.extractBulkErrors(bulkResponse.Items)
		return fmt.Errorf("bulk indexing had errors: %s", errorDetails)
	}

	e.logger.Debug().
		Int("entries", len(entries)).
		Str("index", e.indexName).
		Str("url", e.bulkURL).
		Msg("Sent entries to Elasticsearch")

	return nil
}



func (e *ElasticsearchAdapter) getIndexName(entry *ingestion.LogEntry) string {
	if entry.Labels != nil {
		if index, ok := entry.Labels["index"]; ok && index != "" {
			return index
		}
	}

	date := entry.Timestamp.Format("2006.01.02")
	return fmt.Sprintf("%s-%s", e.indexName, date)
}

func (e *ElasticsearchAdapter) Name() string {
	return e.config.Name
}

// extractBulkErrors extracts detailed error information from bulk response
func (e *ElasticsearchAdapter) extractBulkErrors(items []map[string]interface{}) string {
	var errors []string
	
	for i, item := range items {
		for operation, details := range item {
			if detailsMap, ok := details.(map[string]interface{}); ok {
				if errorInfo, hasError := detailsMap["error"]; hasError {
					errors = append(errors, fmt.Sprintf("Item %d (%s): %v", i, operation, errorInfo))
				}
			}
		}
	}
	
	if len(errors) > 5 {
		errors = errors[:5] // Limit to first 5 errors
		errors = append(errors, "... and more errors")
	}
	
	return strings.Join(errors, "; ")
}

// convertToESDocument converts LogSieve entry to ECS-compliant Elasticsearch document
func (e *ElasticsearchAdapter) convertToESDocument(entry *ingestion.LogEntry) ESDocument {
	doc := ESDocument{
		Timestamp: entry.Timestamp,
		Message:   entry.Message,
		Level:     entry.Level,
		Source:    entry.Source,
		ContainerName: entry.ContainerName,
		ContainerID:   entry.ContainerID,
		PodName:       entry.PodName,
		Namespace:     entry.Namespace,
		NodeName:      entry.NodeName,
		Labels:        entry.Labels,
	}
	
	// Extract host information from labels
	if entry.Labels != nil {
		if hostName, ok := entry.Labels["host_name"]; ok {
			doc.Host.Name = hostName
		}
		if hostIP, ok := entry.Labels["host_ip"]; ok {
			doc.Host.IP = hostIP
		}
		if serviceName, ok := entry.Labels["service_name"]; ok {
			doc.Service.Name = serviceName
		}
		if serviceVersion, ok := entry.Labels["service_version"]; ok {
			doc.Service.Version = serviceVersion
		}
	}
	
	return doc
}

func (e *ElasticsearchAdapter) Close() error {
	return nil
}