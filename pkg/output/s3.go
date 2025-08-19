package output

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
)

type S3Adapter struct {
	config config.OutputConfig
	logger zerolog.Logger
}

func NewS3Adapter(config config.OutputConfig, logger zerolog.Logger) (*S3Adapter, error) {
	return &S3Adapter{
		config: config,
		logger: logger.With().Str("adapter", "s3").Logger(),
	}, nil
}

func (s *S3Adapter) Send(entries []*ingestion.LogEntry) error {
	s.logger.Info().
		Int("entries", len(entries)).
		Msg("S3 adapter not fully implemented - would send to S3")

	var buf bytes.Buffer
	
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal entry: %w", err)
		}
		
		buf.Write(data)
		buf.WriteByte('\n')
	}

	s.logger.Debug().
		Int("entries", len(entries)).
		Int("bytes", buf.Len()).
		Msg("Would upload to S3")

	return nil
}

func (s *S3Adapter) Name() string {
	return s.config.Name
}

func (s *S3Adapter) Close() error {
	return nil
}