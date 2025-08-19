package config

import (
	"time"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Ingestion  IngestionConfig  `mapstructure:"ingestion"`
	Dedup      DedupConfig      `mapstructure:"dedup"`
	Profiles   ProfilesConfig   `mapstructure:"profiles"`
	Outputs    []OutputConfig   `mapstructure:"outputs"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Logging    LoggingConfig    `mapstructure:"logging"`
}

type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	Address      string        `mapstructure:"address"`
	ReadTimeout  time.Duration `mapstructure:"readTimeout"`
	WriteTimeout time.Duration `mapstructure:"writeTimeout"`
	IdleTimeout  time.Duration `mapstructure:"idleTimeout"`
}

type IngestionConfig struct {
	MaxBatchSize   int           `mapstructure:"maxBatchSize"`
	FlushInterval  time.Duration `mapstructure:"flushInterval"`
	MaxMemoryMB    int           `mapstructure:"maxMemoryMB"`
	BufferSize     int           `mapstructure:"bufferSize"`
	MaxRequestSize int64         `mapstructure:"maxRequestSize"`
}

type DedupConfig struct {
	Engine         string        `mapstructure:"engine"`
	CacheSize      int           `mapstructure:"cacheSize"`
	ContextLines   int           `mapstructure:"contextLines"`
	SimilarityThreshold float64  `mapstructure:"similarityThreshold"`
	PatternTTL     time.Duration `mapstructure:"patternTTL"`
	FingerprintTTL time.Duration `mapstructure:"fingerprintTTL"`
}

type ProfilesConfig struct {
	AutoDetect     bool          `mapstructure:"autoDetect"`
	HubURL         string        `mapstructure:"hubURL"`
	SyncInterval   time.Duration `mapstructure:"syncInterval"`
	LocalPath      string        `mapstructure:"localPath"`
	CachePath      string        `mapstructure:"cachePath"`
	DefaultProfile string        `mapstructure:"defaultProfile"`
}

type OutputConfig struct {
	Name      string            `mapstructure:"name"`
	Type      string            `mapstructure:"type"`
	URL       string            `mapstructure:"url"`
	BatchSize int               `mapstructure:"batchSize"`
	Timeout   time.Duration     `mapstructure:"timeout"`
	Retries   int               `mapstructure:"retries"`
	Headers   map[string]string `mapstructure:"headers"`
	Config    map[string]interface{} `mapstructure:"config"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	Structured bool   `mapstructure:"structured"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         8080,
			Address:      "0.0.0.0",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Ingestion: IngestionConfig{
			MaxBatchSize:   1000,
			FlushInterval:  5 * time.Second,
			MaxMemoryMB:    100,
			BufferSize:     10000,
			MaxRequestSize: 10 * 1024 * 1024, // 10MB
		},
		Dedup: DedupConfig{
			Engine:              "drain3",
			CacheSize:           10000,
			ContextLines:        5,
			SimilarityThreshold: 0.9,
			PatternTTL:          1 * time.Hour,
			FingerprintTTL:      30 * time.Minute,
		},
		Profiles: ProfilesConfig{
			AutoDetect:     true,
			HubURL:         "https://hub.logsieve.io",
			SyncInterval:   1 * time.Hour,
			LocalPath:      "/etc/logsieve/profiles",
			CachePath:      "/var/cache/logsieve/profiles",
			DefaultProfile: "generic",
		},
		Outputs: []OutputConfig{
			{
				Name:      "stdout",
				Type:      "stdout",
				BatchSize: 100,
				Timeout:   10 * time.Second,
				Retries:   3,
			},
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			Output:     "stdout",
			Structured: true,
		},
	}
}