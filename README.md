# LogSieve

**Slash your Datadog/Splunk bill by 90% before logs ever leave your cluster.**

LogSieve is a high-performance, self-hosted log deduplication sidecar. It sits between your log collector (like Fluent Bit) and your expensive storage backend (Datadog, Splunk, Loki, Elasticsearch). Using the production-grade **Drain3 algorithm**, it intelligently groups, templates, and drops redundant logs (like health checks, repetitive stack traces, and heartbeats) while preserving the exact context you need for debugging.

## Why LogSieve?

If you are paying $10k/month for log ingestion, $9k of that is likely noise. You don't need to index the exact same `GET /health` log 10,000 times a second.

- **Zero Data Lock-in:** Run LogSieve purely as a sidecar or daemonset in your own AWS/GCP cluster. Your logs never touch our servers.
- **No Regex Maintenance:** Stop writing brittle regex rules. LogSieve's Drain3 engine automatically clusters and templates logs on the fly.
- **Context Preservation:** When an `ERROR` or `FATAL` log occurs, LogSieve automatically preserves the previous N lines of context, ensuring you never lose the exact request trace that caused the crash.

---

## How It Works

```text
┌──────────┐     ┌────────────┐     ┌──────────┐     ┌─────────────────────┐
│ Your App │────>│ Fluent Bit │────>│ LogSieve │────>│ Datadog/Splunk/Loki │
└──────────┘     └────────────┘     └──────────┘     └─────────────────────┘
                                          │
                                    ┌─────┴─────┐
                                    │ Profiles  │
                                    │ (Nginx,   │
                                    │ Postgres, │
                                    │ Custom)   │
                                    └───────────┘
```

1. **Collect:** Fluent Bit sends raw JSON logs to LogSieve's high-throughput HTTP ingestion endpoint.
2. **Deduplicate:** LogSieve applies profile-based rules. Exact duplicates are dropped via SHA256 caching. Similar logs are clustered into templates via Drain3.
3. **Route:** Only unique, critical, or sampled logs are forwarded to your final storage destination.

---

## Features

- **Blazing Fast:** Written in Go. Handles 10,000+ logs/second per instance with sub-10ms p99 latency.
- **Built-in Profiles:** Ships with pre-configured rules for Nginx, PostgreSQL, and Java Spring.
- **Observability Native:** Exposes Prometheus metrics (`logsieve_dedup_ratio`) so you can track exactly how much money you are saving in real-time.
- **Graceful Output Adapters:** Native integrations for Loki, Elasticsearch, and `stdout`.

## Quick Start (Docker Compose)

Deploy LogSieve locally with Fluent Bit, Loki, and Grafana to see the deduplication in action.

```yaml
version: '3.8'

services:
  logsieve:
    image: logsieve/sieve:latest
    ports:
      - "8080:8080"   # HTTP ingestion
      - "9090:9090"   # Prometheus Metrics
    environment:
      - LOGSIEVE_LICENSE_KEY=your_license_key_here
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

Send a test batch:
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

## Configuration

LogSieve is configured via a YAML file.

```yaml
server:
  port: 8080

ingestion:
  maxBatchSize: 1000
  flushInterval: 5s

dedup:
  engine: "drain3"
  cacheSize: 10000
  contextLines: 5
  similarityThreshold: 0.4

outputs:
  - name: "loki"
    type: "loki"
    url: "http://loki:3100"
```

## Architecture

- `cmd/server/`: The HTTP ingestion pipeline and routing engine.
- `pkg/dedup/`: The core Drain3 algorithm and context window management.
- `pkg/profiles/`: Auto-detection and parsing of application-specific rule sets.
- `pkg/output/`: Delivery adapters for external storage.

## License & Pricing

LogSieve is distributed as a commercial binary/Docker container. To use LogSieve in production, you must purchase a license key and provide it via the `LOGSIEVE_LICENSE_KEY` environment variable. 

[Link to Pricing/Landing Page coming soon]