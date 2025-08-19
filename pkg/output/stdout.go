package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
)

type StdoutAdapter struct {
	config config.OutputConfig
	logger zerolog.Logger
}

func NewStdoutAdapter(config config.OutputConfig, logger zerolog.Logger) *StdoutAdapter {
	return &StdoutAdapter{
		config: config,
		logger: logger.With().Str("adapter", "stdout").Logger(),
	}
}

func (s *StdoutAdapter) Send(entries []*ingestion.LogEntry) error {
	for _, entry := range entries {
		output := map[string]interface{}{
			"timestamp":      entry.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
			"message":        entry.Message,
			"level":          entry.Level,
			"source":         entry.Source,
			"container_name": entry.ContainerName,
			"pod_name":       entry.PodName,
			"namespace":      entry.Namespace,
			"labels":         entry.Labels,
		}

		data, err := json.Marshal(output)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to marshal log entry")
			return err
		}

		fmt.Fprintln(os.Stdout, string(data))
	}

	return nil
}

func (s *StdoutAdapter) Name() string {
	return s.config.Name
}

func (s *StdoutAdapter) Close() error {
	return nil
}