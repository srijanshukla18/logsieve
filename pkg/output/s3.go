package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"

	cfg "github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

type S3Adapter struct {
	outputConfig cfg.OutputConfig
	logger       zerolog.Logger
	client       *s3.Client
	bucket       string
	prefix       string
	region       string
	endpoint     string
	metrics      *metrics.Registry

	batchSize    int
	buffer       []*ingestion.LogEntry
	bufferMu     sync.Mutex
	flushTicker  *time.Ticker
	stopCh       chan struct{}
	wg           sync.WaitGroup

	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

type S3Config struct {
	Bucket          string `mapstructure:"bucket"`
	Region          string `mapstructure:"region"`
	Prefix          string `mapstructure:"prefix"`
	Endpoint        string `mapstructure:"endpoint"`
	AccessKeyID     string `mapstructure:"accessKeyId"`
	SecretAccessKey string `mapstructure:"secretAccessKey"`
	BatchSize       int    `mapstructure:"batchSize"`
	FlushInterval   string `mapstructure:"flushInterval"`
	MaxRetries      int    `mapstructure:"maxRetries"`
	UsePathStyle    bool   `mapstructure:"usePathStyle"`
}

func NewS3Adapter(outputConfig cfg.OutputConfig, metricsRegistry *metrics.Registry, logger zerolog.Logger) (*S3Adapter, error) {
	s3cfg := parseS3Config(outputConfig)

	if s3cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}

	if s3cfg.Region == "" {
		s3cfg.Region = "us-east-1"
	}

	ctx := context.Background()
	var awsCfg aws.Config
	var err error

	optFns := []func(*config.LoadOptions) error{
		config.WithRegion(s3cfg.Region),
	}

	if s3cfg.AccessKeyID != "" && s3cfg.SecretAccessKey != "" {
		optFns = append(optFns, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(s3cfg.AccessKeyID, s3cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err = config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if s3cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s3cfg.Endpoint)
			o.UsePathStyle = s3cfg.UsePathStyle
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	batchSize := s3cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	maxRetries := s3cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	flushInterval := 30 * time.Second
	if s3cfg.FlushInterval != "" {
		if parsed, err := time.ParseDuration(s3cfg.FlushInterval); err == nil {
			flushInterval = parsed
		}
	}

	adapter := &S3Adapter{
		outputConfig:   outputConfig,
		logger:         logger.With().Str("adapter", "s3").Str("bucket", s3cfg.Bucket).Logger(),
		client:         client,
		bucket:         s3cfg.Bucket,
		prefix:         s3cfg.Prefix,
		region:         s3cfg.Region,
		endpoint:       s3cfg.Endpoint,
		metrics:        metricsRegistry,
		batchSize:      batchSize,
		buffer:         make([]*ingestion.LogEntry, 0, batchSize),
		stopCh:         make(chan struct{}),
		maxRetries:     maxRetries,
		initialBackoff: 250 * time.Millisecond,
		maxBackoff:     10 * time.Second,
	}

	adapter.flushTicker = time.NewTicker(flushInterval)
	adapter.wg.Add(1)
	go adapter.flushLoop()

	adapter.logger.Info().
		Str("bucket", s3cfg.Bucket).
		Str("region", s3cfg.Region).
		Str("prefix", s3cfg.Prefix).
		Int("batchSize", batchSize).
		Dur("flushInterval", flushInterval).
		Msg("S3 adapter initialized")

	return adapter, nil
}

func parseS3Config(outputConfig cfg.OutputConfig) S3Config {
	s3cfg := S3Config{
		BatchSize:  outputConfig.BatchSize,
		MaxRetries: outputConfig.Retries,
	}

	if outputConfig.Config != nil {
		if v, ok := outputConfig.Config["bucket"].(string); ok {
			s3cfg.Bucket = v
		}
		if v, ok := outputConfig.Config["region"].(string); ok {
			s3cfg.Region = v
		}
		if v, ok := outputConfig.Config["prefix"].(string); ok {
			s3cfg.Prefix = v
		}
		if v, ok := outputConfig.Config["endpoint"].(string); ok {
			s3cfg.Endpoint = v
		}
		if v, ok := outputConfig.Config["accessKeyId"].(string); ok {
			s3cfg.AccessKeyID = v
		}
		if v, ok := outputConfig.Config["secretAccessKey"].(string); ok {
			s3cfg.SecretAccessKey = v
		}
		if v, ok := outputConfig.Config["flushInterval"].(string); ok {
			s3cfg.FlushInterval = v
		}
		if v, ok := outputConfig.Config["usePathStyle"].(bool); ok {
			s3cfg.UsePathStyle = v
		}
	}

	return s3cfg
}

func (s *S3Adapter) Send(entries []*ingestion.LogEntry) error {
	s.bufferMu.Lock()
	s.buffer = append(s.buffer, entries...)

	if len(s.buffer) >= s.batchSize {
		toFlush := s.buffer
		s.buffer = make([]*ingestion.LogEntry, 0, s.batchSize)
		s.bufferMu.Unlock()
		return s.uploadBatch(toFlush)
	}

	s.bufferMu.Unlock()
	return nil
}

func (s *S3Adapter) flushLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.flushTicker.C:
			s.bufferMu.Lock()
			if len(s.buffer) > 0 {
				toFlush := s.buffer
				s.buffer = make([]*ingestion.LogEntry, 0, s.batchSize)
				s.bufferMu.Unlock()
				if err := s.uploadBatch(toFlush); err != nil {
					s.logger.Error().Err(err).Int("entries", len(toFlush)).Msg("Failed to flush buffer to S3")
				}
			} else {
				s.bufferMu.Unlock()
			}
		case <-s.stopCh:
			s.bufferMu.Lock()
			if len(s.buffer) > 0 {
				toFlush := s.buffer
				s.buffer = nil
				s.bufferMu.Unlock()
				if err := s.uploadBatch(toFlush); err != nil {
					s.logger.Error().Err(err).Int("entries", len(toFlush)).Msg("Failed to flush remaining buffer on shutdown")
				}
			} else {
				s.bufferMu.Unlock()
			}
			return
		}
	}
}

func (s *S3Adapter) uploadBatch(entries []*ingestion.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	start := time.Now()
	ctx := context.Background()

	var buf bytes.Buffer
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to marshal entry, skipping")
			continue
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}

	key := s.generateKey()
	contentType := "application/x-ndjson"

	var lastErr error
	backoff := s.initialBackoff

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			s.logger.Debug().
				Int("attempt", attempt).
				Dur("backoff", backoff).
				Str("key", key).
				Msg("Retrying S3 upload")
			time.Sleep(backoff)
			backoff = backoff * 2
			if backoff > s.maxBackoff {
				backoff = s.maxBackoff
			}
		}

		_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(s.bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(buf.Bytes()),
			ContentType: aws.String(contentType),
		})

		if err == nil {
			duration := time.Since(start)
			s.logger.Debug().
				Str("key", key).
				Int("entries", len(entries)).
				Int("bytes", buf.Len()).
				Dur("duration", duration).
				Msg("Uploaded batch to S3")

			if s.metrics != nil {
				s.metrics.OutputLogsTotal.WithLabelValues(s.outputConfig.Name, "success").Add(float64(len(entries)))
				s.metrics.OutputBytesTotal.WithLabelValues(s.outputConfig.Name).Add(float64(buf.Len()))
				s.metrics.OutputDuration.WithLabelValues(s.outputConfig.Name).Observe(duration.Seconds())
			}
			return nil
		}

		lastErr = err
		s.logger.Warn().
			Err(err).
			Int("attempt", attempt+1).
			Int("maxRetries", s.maxRetries).
			Str("key", key).
			Msg("S3 upload failed")
	}

	if s.metrics != nil {
		s.metrics.OutputErrorsTotal.WithLabelValues(s.outputConfig.Name, "upload_failed").Inc()
	}

	return fmt.Errorf("failed to upload to S3 after %d attempts: %w", s.maxRetries+1, lastErr)
}

func (s *S3Adapter) generateKey() string {
	now := time.Now().UTC()

	var key strings.Builder
	if s.prefix != "" {
		key.WriteString(strings.TrimSuffix(s.prefix, "/"))
		key.WriteString("/")
	}

	key.WriteString(fmt.Sprintf("year=%d/month=%02d/day=%02d/hour=%02d/",
		now.Year(), now.Month(), now.Day(), now.Hour()))

	key.WriteString(fmt.Sprintf("logsieve-%s.jsonl", now.Format("20060102-150405.000")))

	return key.String()
}

func (s *S3Adapter) Name() string {
	return s.outputConfig.Name
}

func (s *S3Adapter) Close() error {
	s.logger.Info().Msg("Shutting down S3 adapter")

	s.flushTicker.Stop()
	close(s.stopCh)
	s.wg.Wait()

	s.logger.Info().Msg("S3 adapter shutdown complete")
	return nil
}
