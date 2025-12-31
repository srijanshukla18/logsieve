# LogSieve

A high-performance log deduplication sidecar that reduces container log volumes by ~90% using the Drain3 algorithm and community-powered profiles.

## Why

**Your logging bill is 90% noise.** Health checks, heartbeats, duplicate stack traces - they drown the signal while exploding storage costs. LogSieve sits between your log collector and storage backend, intelligently deduplicating logs using shared profiles so you don't have to write (and maintain) regex filters for every application in your stack.

---

## How It Works

```
┌──────────┐     ┌────────────┐     ┌──────────┐     ┌─────────────────┐
│ Your App │────>│ Fluent Bit │────>│ LogSieve │────>│ Loki/ES/S3/etc  │
└──────────┘     └────────────┘     └──────────┘     └─────────────────┘
                                          │
                                    ┌─────┴─────┐
                                    │ Profiles  │
                                    │ (builtin  │
                                    │ + custom) │
                                    └───────────┘
```

1. **Fluent Bit** (or any log collector) sends logs to LogSieve's HTTP endpoint
2. **LogSieve** applies profile-based rules and Drain3 templating to deduplicate
3. **Only unique/important logs** get forwarded to your storage backend

---

## Features

### Deduplication Engine
- **Drain3 Algorithm**: Production-grade implementation with prefix tree clustering, template generalization, and configurable similarity thresholds
- **Fingerprint Cache**: SHA256-based exact duplicate detection with TTL
- **Context Windows**: Preserve N lines around errors/critical events

### Profile System
- **Auto-detection**: Matches containers to profiles by image name or log patterns
- **Builtin Profiles**: nginx, postgres, java-spring included out of the box
- **YAML Configuration**: Define fingerprint rules, sampling rates, transforms, and routing
- **Signature Verification**: Ed25519 signing support for profile integrity (strict/relaxed modes)

### Output Adapters
- **Loki**: Full v3+ support with structured metadata and cardinality-aware label handling
- **Elasticsearch**: ECS-compliant document structure
- **S3**: Batched uploads for archival
- **stdout**: Development and debugging

### Observability
- **Prometheus Metrics**: `logsieve_dedup_ratio`, `logsieve_ingestion_logs_total`, `logsieve_output_errors_total`, etc.
- **Circuit Breakers**: Automatic backoff and retry with exponential delays
- **Health Endpoints**: `/health`, `/ready`, `/stats`

---

## Quick Start

### Docker Compose

```yaml
services:
  logsieve:
    image: logsieve/sieve:latest
    ports:
      - "8080:8080"   # HTTP ingestion
      - "9090:9090"   # Metrics
    environment:
      - LOGSIEVE_PROFILES_AUTODETECT=true
    volumes:
      - ./config.yaml:/etc/logsieve/config.yaml

  fluent-bit:
    image: fluent/fluent-bit:2.1
    volumes:
      - ./fluent-bit.conf:/fluent-bit/etc/fluent-bit.conf
    depends_on:
      - logsieve
```

### Fluent Bit Configuration

```ini
[OUTPUT]
    Name          http
    Match         *
    Host          logsieve
    Port          8080
    URI           /ingest?profile=auto
    Format        json
    Header        X-Source fluent-bit
```

### Send Test Logs

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "logs": [
      {"log": "User 123 logged in from 192.168.1.1", "time": "2024-01-01T00:00:00Z"},
      {"log": "User 456 logged in from 10.0.0.1", "time": "2024-01-01T00:00:01Z"}
    ]
  }'
```

---

## Configuration

```yaml
server:
  port: 8080
  address: "0.0.0.0"

ingestion:
  maxBatchSize: 1000
  flushInterval: 5s
  maxMemoryMB: 100
  queueType: memory  # or "disk" for persistence

dedup:
  engine: "drain3"
  cacheSize: 10000
  contextLines: 5
  similarityThreshold: 0.4

profiles:
  autoDetect: true
  localPath: "/etc/logsieve/profiles"
  trustMode: relaxed  # strict|relaxed|offline

outputs:
  - name: "loki"
    type: "loki"
    url: "http://loki:3100"
    batchSize: 100
    retries: 3

metrics:
  enabled: true
  port: 9090
```

---

## Profiles

Profiles define how logs from specific applications should be processed. Example for nginx:

```yaml
apiVersion: hub.logsieve.io/v1
kind: LogProfile
metadata:
  name: nginx
  version: "1.0.0"
  images:
    - "nginx:*"

spec:
  fingerprints:
    - pattern: '"GET /health'
      action: "drop"
    - pattern: '\[error\]'
      action: "keep"
    - pattern: '\d+\.\d+\.\d+\.\d+ - - \[.*?\]'
      action: "template"

  sampling:
    - pattern: '"GET /health'
      rate: 0.01  # Keep 1%

  transforms:
    - field: "message"
      regex: '(password=)[^&\s]+'
      replace: '$1***'
```

### Profile Actions
- **drop**: Discard the log entirely
- **keep**: Always forward (bypass deduplication)
- **template**: Apply Drain3 clustering

---

## Architecture

```
cmd/
  logsieve/       # CLI tool (capture, learn, audit, config)
  server/         # HTTP server entry point

pkg/
  config/         # YAML configuration loading
  dedup/          # Drain3 + fingerprint + context window
  ingestion/      # HTTP handler, parser, memory/disk buffer
  metrics/        # Prometheus registry
  output/         # Loki, Elasticsearch, S3, stdout adapters
  processor/      # Orchestrates dedup -> profiles -> routing
  profiles/       # Profile parsing, detection, verification

profiles/         # Builtin YAML profiles (nginx, postgres, java-spring)
docker/           # Dockerfile and Dockerfile.distroless
```

---

## CLI Commands

```bash
# Start the server
logsieve server --config config.yaml

# Generate example config
logsieve config example -o config.yaml

# Validate config
logsieve config validate -c config.yaml

# Show version
logsieve version
```

---

## Metrics

| Metric | Description |
|--------|-------------|
| `logsieve_dedup_ratio` | Deduplication ratio (0-1) per profile |
| `logsieve_ingestion_logs_total` | Total logs ingested by source/profile |
| `logsieve_output_logs_total` | Logs forwarded to outputs |
| `logsieve_output_errors_total` | Output delivery failures |
| `logsieve_dedup_patterns_total` | Active Drain3 clusters |

---

## Building from Source

```bash
# Prerequisites: Go 1.21+
git clone https://github.com/logsieve/logsieve.git
cd logsieve

# Build
make build

# Run tests
make test

# Build Docker image
make docker-build
```

---

## Performance Targets

- **Throughput**: 10,000 logs/second per instance
- **Latency**: <10ms p99
- **Memory**: <100MB baseline, <500MB under load
- **Dedup Ratio**: >85% for typical workloads

---

## License

[License details to be added]
