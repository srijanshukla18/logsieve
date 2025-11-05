// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
	"github.com/logsieve/logsieve/pkg/processor"
)

func TestEndToEndIngestion(t *testing.T) {
	// Setup
	logger := zerolog.Nop()
	cfg := config.DefaultConfig()
	metricsRegistry := metrics.NewRegistry()

	proc, err := processor.NewProcessor(cfg, metricsRegistry, logger)
	require.NoError(t, err)

	handler := ingestion.NewHandler(cfg, metricsRegistry, logger)
	handler.SetProcessor(proc)

	// Test data
	requestBody := map[string]interface{}{
		"logs": []map[string]interface{}{
			{
				"log":  "Test log message 1",
				"time": time.Now().Format(time.RFC3339),
			},
			{
				"log":  "Test log message 2",
				"time": time.Now().Format(time.RFC3339),
			},
		},
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute
	handler.HandleIngest(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, float64(2), response["processed"])
}

func TestHealthCheck(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Simple health check handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteStatus(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "healthy",
			"time":   time.Now().UTC(),
		})
	})

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
