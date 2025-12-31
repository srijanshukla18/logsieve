package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func Load(configPath string) (*Config, error) {
	v := viper.New()
	
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/logsieve")
		v.AddConfigPath("$HOME/.logsieve")
	}

	v.SetEnvPrefix("LOGSIEVE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		} else {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	config := DefaultConfig()
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

func setDefaults(v *viper.Viper) {
	defaults := DefaultConfig()
	
	v.SetDefault("server.port", defaults.Server.Port)
	v.SetDefault("server.address", defaults.Server.Address)
	v.SetDefault("server.readTimeout", defaults.Server.ReadTimeout)
	v.SetDefault("server.writeTimeout", defaults.Server.WriteTimeout)
	v.SetDefault("server.idleTimeout", defaults.Server.IdleTimeout)
	
	v.SetDefault("ingestion.maxBatchSize", defaults.Ingestion.MaxBatchSize)
	v.SetDefault("ingestion.flushInterval", defaults.Ingestion.FlushInterval)
	v.SetDefault("ingestion.maxMemoryMB", defaults.Ingestion.MaxMemoryMB)
	v.SetDefault("ingestion.bufferSize", defaults.Ingestion.BufferSize)
    v.SetDefault("ingestion.maxRequestSize", defaults.Ingestion.MaxRequestSize)
    v.SetDefault("ingestion.queueType", defaults.Ingestion.QueueType)
    v.SetDefault("ingestion.diskPath", defaults.Ingestion.DiskPath)
    v.SetDefault("ingestion.maxDiskBytes", defaults.Ingestion.MaxDiskBytes)
	
	v.SetDefault("dedup.engine", defaults.Dedup.Engine)
	v.SetDefault("dedup.cacheSize", defaults.Dedup.CacheSize)
	v.SetDefault("dedup.contextLines", defaults.Dedup.ContextLines)
	v.SetDefault("dedup.similarityThreshold", defaults.Dedup.SimilarityThreshold)
	v.SetDefault("dedup.patternTTL", defaults.Dedup.PatternTTL)
	v.SetDefault("dedup.fingerprintTTL", defaults.Dedup.FingerprintTTL)
	
	v.SetDefault("profiles.autoDetect", defaults.Profiles.AutoDetect)
	v.SetDefault("profiles.hubURL", defaults.Profiles.HubURL)
	v.SetDefault("profiles.syncInterval", defaults.Profiles.SyncInterval)
	v.SetDefault("profiles.localPath", defaults.Profiles.LocalPath)
	v.SetDefault("profiles.cachePath", defaults.Profiles.CachePath)
    v.SetDefault("profiles.defaultProfile", defaults.Profiles.DefaultProfile)
    v.SetDefault("profiles.trustMode", defaults.Profiles.TrustMode)
    v.SetDefault("profiles.publicKeys", defaults.Profiles.PublicKeys)
	
	v.SetDefault("metrics.enabled", defaults.Metrics.Enabled)
	v.SetDefault("metrics.port", defaults.Metrics.Port)
	v.SetDefault("metrics.path", defaults.Metrics.Path)
	
	v.SetDefault("logging.level", defaults.Logging.Level)
	v.SetDefault("logging.format", defaults.Logging.Format)
	v.SetDefault("logging.output", defaults.Logging.Output)
	v.SetDefault("logging.structured", defaults.Logging.Structured)
}

func validateConfig(config *Config) error {
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}
	
	if config.Metrics.Enabled && (config.Metrics.Port <= 0 || config.Metrics.Port > 65535) {
		return fmt.Errorf("invalid metrics port: %d", config.Metrics.Port)
	}
	
	if config.Ingestion.MaxBatchSize <= 0 {
		return fmt.Errorf("invalid maxBatchSize: %d", config.Ingestion.MaxBatchSize)
	}
	
	if config.Dedup.SimilarityThreshold < 0 || config.Dedup.SimilarityThreshold > 1 {
		return fmt.Errorf("invalid similarity threshold: %f", config.Dedup.SimilarityThreshold)
	}
	
	validEngines := map[string]bool{
		"drain3": true,
		"simple": true,
	}
	if !validEngines[config.Dedup.Engine] {
		return fmt.Errorf("invalid dedup engine: %s", config.Dedup.Engine)
	}
	
	validLogLevels := map[string]bool{
		"trace": true,
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
		"fatal": true,
		"panic": true,
	}
	if !validLogLevels[strings.ToLower(config.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", config.Logging.Level)
	}
	
    for i, output := range config.Outputs {
		if output.Name == "" {
			return fmt.Errorf("output %d: name is required", i)
		}
		if output.Type == "" {
			return fmt.Errorf("output %d (%s): type is required", i, output.Name)
		}
		if output.BatchSize <= 0 {
			config.Outputs[i].BatchSize = 100
		}
		if output.Timeout == 0 {
			config.Outputs[i].Timeout = 10 * config.Server.WriteTimeout
		}
        if output.Retries <= 0 {
            config.Outputs[i].Retries = 3
        }
        if output.InitialBackoff <= 0 {
            config.Outputs[i].InitialBackoff = 250 * time.Millisecond
        }
        if output.MaxBackoff <= 0 {
            config.Outputs[i].MaxBackoff = 5 * time.Second
        }
        if output.MaxFailures <= 0 {
            config.Outputs[i].MaxFailures = 5
        }
        if output.Cooldown <= 0 {
            config.Outputs[i].Cooldown = 30 * time.Second
        }
    }
	
	return nil
}

func WriteExample(path string) error {
	exampleConfig := `# LogSieve Configuration Example

server:
  port: 8080
  address: "0.0.0.0"
  readTimeout: 30s
  writeTimeout: 30s
  idleTimeout: 60s

ingestion:
  maxBatchSize: 1000
  flushInterval: 5s
  maxMemoryMB: 100
  bufferSize: 10000
  maxRequestSize: 10485760  # 10MB

dedup:
  engine: "drain3"
  cacheSize: 10000
  contextLines: 5
  similarityThreshold: 0.9
  patternTTL: 1h
  fingerprintTTL: 30m

profiles:
  autoDetect: true
  hubURL: "https://hub.logsieve.io"
  syncInterval: 1h
  localPath: "/etc/logsieve/profiles"
  cachePath: "/var/cache/logsieve/profiles"
  defaultProfile: "generic"

outputs:
  - name: "loki"
    type: "loki"
    url: "http://loki:3100"
    batchSize: 100
    timeout: 10s
    retries: 3
    headers:
      "Content-Type": "application/json"
  
  - name: "elasticsearch"
    type: "elasticsearch"
    url: "http://elasticsearch:9200"
    batchSize: 50
    timeout: 15s
    retries: 3
    config:
      index: "logs"
      template: "logs-template"

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

logging:
  level: "info"
  format: "json"
  output: "stdout"
  structured: true
`

	return os.WriteFile(path, []byte(exampleConfig), 0644)
}
