# Getting Started with LogSieve

LogSieve is a high-performance log deduplication and filtering system that reduces log volumes by ~90% using community-powered profiles. This guide will get you up and running in minutes.

## Quick Start Options

Choose your deployment method:

- [🐳 Docker Compose](#docker-compose-quickest) - Fastest way to try LogSieve
- [☸️ Kubernetes](#kubernetes-production-ready) - Production deployment
- [🔧 Local Development](#local-development) - Build from source

---

## 🐳 Docker Compose (Quickest)

Perfect for testing LogSieve with your existing logs.

### 1. Create Docker Compose Setup

```bash
mkdir logsieve-demo && cd logsieve-demo
```

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  # Your application (example: nginx)
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    logging:
      driver: "json-file"
      options:
        max-size: "10m"

  # Fluent Bit - collects logs and sends to LogSieve
  fluent-bit:
    image: fluent/fluent-bit:2.1
    volumes:
      - ./fluent-bit.conf:/fluent-bit/etc/fluent-bit.conf
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
      - /var/log:/var/log:ro
    depends_on:
      - logsieve

  # LogSieve - deduplicates and filters logs
  logsieve:
    image: logsieve/sieve:latest
    ports:
      - "8080:8080"  # HTTP ingestion
      - "9090:9090"  # Metrics
    environment:
      - LOGSIEVE_PROFILES_AUTODETECT=true
      - LOGSIEVE_OUTPUTS_0_TYPE=loki
      - LOGSIEVE_OUTPUTS_0_URL=http://loki:3100
    volumes:
      - ./logsieve-config.yaml:/etc/logsieve/config.yaml
    depends_on:
      - loki

  # Loki - log storage
  loki:
    image: grafana/loki:latest
    ports:
      - "3100:3100"
    command: -config.file=/etc/loki/local-config.yaml
    volumes:
      - loki-data:/loki

  # Grafana - log visualization
  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana
      - ./grafana-datasources.yml:/etc/grafana/provisioning/datasources/datasources.yml

volumes:
  loki-data:
  grafana-data:
```

### 2. Configure Fluent Bit

Create `fluent-bit.conf`:

```ini
[SERVICE]
    Flush         5
    Daemon        off
    Log_Level     info

[INPUT]
    Name              tail
    Path              /var/log/containers/*.log
    Parser            cri
    Tag               container.*
    Refresh_Interval  5
    Mem_Buf_Limit     50MB

[INPUT]
    Name              tail
    Path              /var/lib/docker/containers/*/*.log
    Parser            json
    Tag               docker.*
    Refresh_Interval  5

[OUTPUT]
    Name          http
    Match         *
    Host          logsieve
    Port          8080
    URI           /ingest?profile=auto
    Format        json
    Retry_Limit   3
    
    # Headers for better processing
    Header        X-Source fluent-bit
    Header        Content-Type application/json
```

### 3. Configure LogSieve

Create `logsieve-config.yaml`:

```yaml
server:
  port: 8080
  address: "0.0.0.0"

ingestion:
  maxBatchSize: 1000
  flushInterval: 5s
  maxMemoryMB: 200

dedup:
  engine: "drain3"
  cacheSize: 10000
  contextLines: 5
  config:
    simThreshold: 0.4
    maxNodeDepth: 4
    maxChildren: 100

profiles:
  autoDetect: true
  localPath: "/etc/logsieve/profiles"

outputs:
  - name: "loki"
    type: "loki"
    url: "http://loki:3100"
    batchSize: 100
    timeout: 10s

metrics:
  enabled: true
  port: 9090

logging:
  level: "info"
  structured: true
```

### 4. Configure Grafana Data Source

Create `grafana-datasources.yml`:

```yaml
apiVersion: 1

datasources:
  - name: Loki
    type: loki
    access: proxy
    url: http://loki:3100
    isDefault: true
```

### 5. Start Everything

```bash
docker-compose up -d
```

### 6. Verify It's Working

```bash
# Check LogSieve health
curl http://localhost:8080/health

# Check metrics
curl http://localhost:9090/metrics | grep logsieve

# Generate some test logs
curl http://localhost:80  # Hit nginx a few times

# View logs in Grafana
open http://localhost:3000  # admin/admin
```

You should see:
- Logs being processed by LogSieve
- Deduplication metrics showing reduction ratios
- Filtered logs in Loki/Grafana

---

## ☸️ Kubernetes (Production Ready)

Deploy LogSieve in Kubernetes with Helm for production use.

### 1. Add Helm Repository

```bash
helm repo add logsieve https://logsieve.github.io/charts
helm repo update
```

### 2. Install with Fluent Bit Integration

```bash
# Basic installation with auto-profile detection
helm install my-logsieve logsieve/logsieve \
  --set fluentbit.enabled=true \
  --set profiles.autoDetect=true \
  --set outputs.loki.url=http://loki-gateway:3100

# Production installation with custom values
helm install my-logsieve logsieve/logsieve \
  --values production-values.yaml
```

### 3. Create Production Values

Create `production-values.yaml`:

```yaml
replicaCount: 3

image:
  repository: logsieve/sieve
  tag: "latest"

resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 256Mi

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

config:
  ingestion:
    maxBatchSize: 2000
    maxMemoryMB: 400
  
  dedup:
    engine: "drain3"
    cacheSize: 20000
    config:
      simThreshold: 0.4
      maxNodeDepth: 4
      maxClusters: 10000
  
  profiles:
    autoDetect: true
    hubURL: "https://hub.logsieve.io"
    syncInterval: 1h
  
  outputs:
    - name: "loki-prod"
      type: "loki"
      url: "http://loki-gateway:3100"
      batchSize: 200
      retries: 5

fluentbit:
  enabled: true
  config:
    inputs:
      - name: tail
        path: /var/log/containers/*.log
        parser: cri
        tag: kube.*
    outputs:
      - name: http
        host: my-logsieve
        port: 8080
        uri: /ingest?profile=auto

monitoring:
  serviceMonitor:
    enabled: true
  grafanaDashboard:
    enabled: true

persistence:
  enabled: true
  size: 10Gi
```

### 4. Verify Kubernetes Deployment

```bash
# Check pods
kubectl get pods -l app=logsieve

# Check logs
kubectl logs -l app=logsieve -f

# Check metrics
kubectl port-forward svc/my-logsieve 9090:9090
curl http://localhost:9090/metrics

# Check ingestion
kubectl port-forward svc/my-logsieve 8080:8080
curl http://localhost:8080/stats
```

---

## 🔧 Local Development

Build and run LogSieve from source for development.

### 1. Prerequisites

```bash
# Install Go 1.21+
go version

# Install dependencies
git clone https://github.com/logsieve/logsieve.git
cd logsieve
go mod download
```

### 2. Build LogSieve

```bash
# Build the server
make build

# Or build with version info
make build VERSION=dev COMMIT=$(git rev-parse HEAD)

# Run tests
make test

# Run benchmarks
make benchmark
```

### 3. Create Development Config

Create `config/development.yaml`:

```yaml
server:
  port: 8080
  address: "localhost"

ingestion:
  maxBatchSize: 500
  flushInterval: 2s

dedup:
  engine: "drain3"
  cacheSize: 5000
  config:
    simThreshold: 0.4
    maxNodeDepth: 3

profiles:
  autoDetect: true
  localPath: "./profiles"

outputs:
  - name: "stdout"
    type: "stdout"

logging:
  level: "debug"
  structured: false

metrics:
  enabled: true
  port: 9090
```

### 4. Run LogSieve

```bash
# Run with development config
./bin/server --config config/development.yaml

# Or run directly with go
go run cmd/server/main.go --config config/development.yaml --log-level debug
```

### 5. Test with Sample Data

```bash
# Send test logs
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "logs": [
      {"log": "User 123 logged in from 192.168.1.1", "time": "2024-01-01T00:00:00Z"},
      {"log": "User 456 logged in from 10.0.0.1", "time": "2024-01-01T00:00:01Z"},
      {"log": "User 789 logged in from 172.16.0.1", "time": "2024-01-01T00:00:02Z"}
    ]
  }'

# Check stats
curl http://localhost:8080/stats

# Check metrics
curl http://localhost:9090/metrics | grep logsieve
```

---

## 📊 Monitoring Your Setup

### Key Metrics to Watch

```bash
# Ingestion rate
logsieve_ingestion_logs_total

# Deduplication effectiveness
logsieve_dedup_ratio

# Output health
logsieve_output_errors_total

# Memory usage
logsieve_drain3_clusters_total
```

### Grafana Dashboard

Import the LogSieve dashboard:

1. Go to Grafana → Dashboards → Import
2. Use dashboard ID: `logsieve-overview`
3. Configure Prometheus data source

### Alerting Rules

```yaml
groups:
  - name: logsieve
    rules:
      - alert: LogSieveDeduplicationLow
        expr: logsieve_dedup_ratio < 0.5
        for: 5m
        annotations:
          summary: "LogSieve deduplication ratio is low"
      
      - alert: LogSieveOutputErrors
        expr: rate(logsieve_output_errors_total[5m]) > 0.1
        for: 2m
        annotations:
          summary: "LogSieve output errors detected"
```

---

## 🎯 Next Steps

### 1. Optimize Your Setup
- Monitor deduplication ratios
- Adjust Drain3 configuration for your log patterns
- Set up proper alerting

### 2. Create Custom Profiles
- Analyze your application logs
- Create profiles for better deduplication
- Share profiles with the community

### 3. Scale for Production
- Use Kubernetes with proper resource limits
- Set up monitoring and alerting
- Configure backup and disaster recovery

### 4. Join the Community
- 📚 [Documentation](https://logsieve.io/docs)
- 💬 [Slack Community](https://slack.logsieve.io)
- 🐛 [GitHub Issues](https://github.com/logsieve/logsieve/issues)
- 🌟 [Profile Hub](https://hub.logsieve.io)

---

## 🆘 Troubleshooting

### Common Issues

**LogSieve not receiving logs:**
```bash
# Check Fluent Bit configuration
kubectl logs -l app=fluent-bit

# Verify LogSieve ingestion endpoint
curl -v http://logsieve:8080/health
```

**Low deduplication ratio:**
```bash
# Check Drain3 configuration
curl http://logsieve:8080/stats

# Review log patterns
kubectl logs -l app=logsieve | grep "unknown pattern"
```

**Output errors:**
```bash
# Check output configuration
curl http://logsieve:9090/metrics | grep output_errors

# Verify target system (Loki/Elasticsearch)
curl http://loki:3100/ready
```

### Getting Help

- Check the [troubleshooting guide](https://logsieve.io/docs/troubleshooting)
- Join our [Slack community](https://slack.logsieve.io)
- Open an issue on [GitHub](https://github.com/logsieve/logsieve/issues)

---

**🎉 You're now ready to reduce your log volumes by 90%!**

Start with the Docker Compose setup to see LogSieve in action, then move to Kubernetes for production deployment.