# LogSieve Practical Testing Guide

## What Actually Works Today (Honest Assessment)

### Fully Functional:
- HTTP server with `/ingest` endpoint for receiving logs
- `/health` and `/ready` endpoints
- `/stats` endpoint for runtime statistics
- Prometheus metrics on separate port (9090)
- Drain3 deduplication algorithm (learns log templates, deduplicates repetitive logs)
- Fingerprint-based exact duplicate detection
- Memory buffer with batch processing
- Stdout output (prints deduplicated logs to console)
- Built-in profiles: `generic` and `nginx`
- Log level extraction from messages
- Multiple timestamp format parsing

### Partially Implemented / Not Tested:
- Loki, Elasticsearch, S3 outputs (code exists but requires external services)
- Disk buffer (code exists, not battle-tested)
- Profile hub sync (hub URL is fake: `hub.logsieve.io`)
- Profile signature verification (code exists, no real keys configured)

### CLI Commands That Are Stubs (Not Implemented):
- `logsieve capture` - Prints "not yet implemented"
- `logsieve learn` - Prints "not yet implemented"
- `logsieve audit` - Prints "not yet implemented"

---

## Prerequisites

### Required:
```bash
# Check Go version (requires Go 1.23+)
go version
# Expected: go1.23 or higher

# Verify the project compiles
cd /Users/srijanshukla/code/projects/active-pending/logsieve
go mod download
```

### Optional (for monitoring):
- `curl` or `httpie` - for sending test requests
- `jq` - for parsing JSON responses

---

## Step 1: Build the Server

```bash
cd /Users/srijanshukla/code/projects/active-pending/logsieve

# Build the server binary
go build -o ./dist/server ./cmd/server

# Verify it built
ls -la ./dist/server
```

**Expected output:**
```
-rwxr-xr-x  1 user  staff  15000000 Dec 31 12:00 ./dist/server
```

---

## Step 2: Create a Minimal Test Config

Create a file `config.test.yaml`:

```bash
cat > config.test.yaml << 'EOF'
server:
  port: 8080
  address: "0.0.0.0"
  readTimeout: 30s
  writeTimeout: 30s
  idleTimeout: 60s

ingestion:
  maxBatchSize: 10
  flushInterval: 2s
  bufferSize: 1000
  maxRequestSize: 10485760

dedup:
  engine: "drain3"
  cacheSize: 1000
  contextLines: 3
  similarityThreshold: 0.4
  patternTTL: 1h
  fingerprintTTL: 30m

profiles:
  autoDetect: true
  localPath: ""
  defaultProfile: "generic"
  trustMode: "offline"

outputs:
  - name: "stdout"
    type: "stdout"
    batchSize: 10
    timeout: 10s
    retries: 1

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

logging:
  level: "debug"
  format: "console"
  output: "stdout"
  structured: false
EOF
```

---

## Step 3: Start the Server

Open a terminal and run:

```bash
cd /Users/srijanshukla/code/projects/active-pending/logsieve
./dist/server --config=config.test.yaml --log-level=debug
```

**Expected startup output:**
```
12:00:00 INF Starting LogSieve server version=dev commit=unknown build_time=unknown
12:00:00 INF Loaded profiles count=2
12:00:00 INF Initialized output adapter output=stdout type=stdout
12:00:00 INF Starting log processor
12:00:00 INF Starting HTTP server address=0.0.0.0:8080
12:00:00 INF Starting metrics server address=:9090 path=/metrics
```

---

## Step 4: Verify Server is Running

In a new terminal:

```bash
# Health check
curl http://localhost:8080/health
```

**Expected output:**
```json
{"status":"healthy","timestamp":"2025-12-31T12:00:00Z","version":"dev"}
```

```bash
# Ready check
curl http://localhost:8080/ready
```

**Expected output:**
```json
{"status":"ready"}
```

---

## Step 5: Send Test Logs (See Deduplication in Action)

### 5a. Send a single log entry

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -H "X-Source: test-app" \
  -d '{"log": "User 12345 logged in successfully", "time": "2025-12-31T12:00:00Z"}'
```

**Expected response:**
```json
{"status":"success","processed":1}
```

**In server terminal, you should see:**
1. Debug logs about processing
2. The deduplicated log printed to stdout (JSON format)

### 5b. Send the same log again (exact duplicate)

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -H "X-Source: test-app" \
  -d '{"log": "User 12345 logged in successfully", "time": "2025-12-31T12:00:01Z"}'
```

**What happens:**
- Response is still `{"status":"success","processed":1}` (processed means received)
- But the log should NOT appear in stdout output (deduplicated via fingerprint cache)

### 5c. Send a similar log (template deduplication)

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -H "X-Source: test-app" \
  -d '{"log": "User 67890 logged in successfully", "time": "2025-12-31T12:00:02Z"}'
```

**What happens:**
- Drain3 recognizes this matches the template "User <NUM> logged in successfully"
- After the first occurrence, similar messages are deduplicated

### 5d. Demonstrate deduplication with batch

```bash
# Send 5 similar logs rapidly
for i in {1..5}; do
  curl -s -X POST http://localhost:8080/ingest \
    -H "Content-Type: application/json" \
    -H "X-Source: test-app" \
    -d "{\"log\": \"Request processed in ${i}00ms\", \"time\": \"2025-12-31T12:00:0${i}Z\"}"
done
```

**Expected behavior:**
- First request creates a new template
- Subsequent similar requests are deduplicated
- You should see fewer than 5 logs in the server stdout output

---

## Step 6: Verify Metrics

```bash
# Get all metrics
curl http://localhost:9090/metrics | grep logsieve

# Key metrics to look for:
curl http://localhost:9090/metrics | grep -E "logsieve_ingestion_logs_total|logsieve_dedup"
```

**Key metrics explained:**

| Metric | What It Shows |
|--------|---------------|
| `logsieve_ingestion_logs_total` | Total logs received |
| `logsieve_dedup_patterns_total` | Number of unique log templates learned |
| `logsieve_dedup_cache_hits_total` | Deduplication hits (fingerprint/template) |
| `logsieve_dedup_ratio` | Current deduplication ratio (0-1) |
| `logsieve_output_logs_total` | Logs actually sent to outputs |

---

## Step 7: Check Runtime Stats

```bash
curl http://localhost:8080/stats | jq .
```

**Expected output structure:**
```json
{
  "running": true,
  "buffer_stats": {
    "buffer_size": 0,
    "buffer_capacity": 1000,
    "batch_queue_size": 0,
    "batch_queue_capacity": 100
  },
  "dedup_stats": {
    "pattern_count": 2,
    "fingerprint_count": 5,
    "context_size": 5,
    "last_processed": "2025-12-31T12:00:00Z"
  },
  "profile_stats": {
    "profile_count": 2,
    "profiles": ["generic", "nginx"]
  },
  "router_stats": {
    "adapter_count": 1,
    "adapters": ["stdout"]
  }
}
```

---

## Step 8: Test with Fluent Bit Format

LogSieve accepts Fluent Bit's output format:

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -H "X-Source: fluent-bit" \
  -d '{
    "log": "2025-12-31 12:00:00 ERROR Connection refused to database server 192.168.1.100",
    "@timestamp": "2025-12-31T12:00:00Z",
    "stream": "stderr",
    "tag": "app.backend",
    "labels": {
      "container_name": "api-server",
      "io.kubernetes.pod.name": "api-server-abc123",
      "io.kubernetes.pod.namespace": "production"
    }
  }'
```

**What happens:**
- Log level is extracted as "ERROR"
- Kubernetes metadata is parsed
- The log is processed through dedup pipeline

---

## Step 9: Test Error Context Preservation

When ERROR logs are detected, LogSieve preserves context:

```bash
# Send some INFO logs
for i in {1..3}; do
  curl -s -X POST http://localhost:8080/ingest \
    -H "Content-Type: application/json" \
    -d "{\"log\": \"Processing request $i\", \"time\": \"2025-12-31T12:00:0${i}Z\"}"
done

# Send an ERROR log
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{"log": "ERROR: Database connection failed after 3 retries", "time": "2025-12-31T12:00:05Z"}'
```

**Expected behavior:**
- The ERROR log should output along with its preceding context lines
- Check the server terminal output

---

## Step 10: Graceful Shutdown

Press `Ctrl+C` in the server terminal.

**Expected shutdown output:**
```
^C
12:00:30 INF Received shutdown signal signal=interrupt
12:00:30 INF Shutting down servers...
12:00:30 INF Stopping log processor
12:00:30 INF Context cancelled, stopping processor
12:00:30 INF Server shutdown completed
```

---

## Quick Verification Checklist

Run these commands and check expected outputs:

| Command | Expected |
|---------|----------|
| `curl localhost:8080/health` | `{"status":"healthy"...}` |
| `curl localhost:8080/ready` | `{"status":"ready"}` |
| `curl localhost:8080/stats` | JSON with running:true |
| `curl localhost:9090/metrics \| grep logsieve` | Multiple metric lines |
| POST to `/ingest` | `{"status":"success"...}` |

---

## Demonstrating Deduplication Effectiveness

Run this test to see real deduplication:

```bash
echo "=== Sending 20 similar logs ==="
for i in {1..20}; do
  curl -s -X POST http://localhost:8080/ingest \
    -H "Content-Type: application/json" \
    -d "{\"log\": \"Connection established to server 10.0.0.$((i % 5)) on port 5432\"}"
done

echo ""
echo "=== Check dedup stats ==="
curl -s http://localhost:8080/stats | jq '.dedup_stats'

echo ""
echo "=== Check metrics ==="
curl -s http://localhost:9090/metrics | grep -E "logsieve_dedup_ratio|logsieve_dedup_patterns_total"
```

**What you should observe:**
- `pattern_count` should be 1 or 2 (not 20)
- Many fewer than 20 logs printed to stdout
- `logsieve_dedup_ratio` should be > 0.5

---

## Troubleshooting

### Server won't start
```bash
# Check if port is in use
lsof -i :8080
lsof -i :9090

# Kill existing processes if needed
kill -9 $(lsof -t -i:8080)
```

### No logs appearing in output
- Wait 2+ seconds (flush interval)
- Check config has `outputs` with `type: "stdout"`
- Check server logs for errors

### Metrics endpoint returns nothing
- Verify metrics port (9090) is correct
- Check `metrics.enabled: true` in config

### "go: cannot find main module" error
```bash
cd /Users/srijanshukla/code/projects/active-pending/logsieve
go mod download
```

---

## What This Proves is Working

1. **HTTP Ingestion** - Server receives logs via POST
2. **Drain3 Algorithm** - Template-based log clustering
3. **Fingerprint Cache** - Exact duplicate detection
4. **Prometheus Metrics** - Observable dedup effectiveness
5. **Batch Processing** - Efficient log batching
6. **Profile System** - Basic profile loading (builtin)
7. **Stdout Output** - Deduplicated logs emitted

---

## What You Cannot Test (Yet)

- Real Loki/Elasticsearch/S3 outputs (need external services)
- Profile hub sync (hub doesn't exist)
- CLI capture/learn/audit commands (stubs only)
- High-volume load testing (use `make load-test` with wrk)
