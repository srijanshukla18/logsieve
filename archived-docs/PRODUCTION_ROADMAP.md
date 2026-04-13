# LogSieve Production Readiness Roadmap

**Audit Date:** 2025-12-31
**Goal:** Make this project "git clone && make && run" ready for ANY engineer

---

## 1. Current State Summary

### What Actually Works TODAY

| Component | Status | Notes |
|-----------|--------|-------|
| HTTP Server (`/ingest`, `/health`, `/ready`, `/stats`) | **WORKING** | Fully functional with Gin framework |
| Prometheus Metrics (`:9090/metrics`) | **WORKING** | 25+ metrics registered |
| Drain3 Deduplication Algorithm | **WORKING** | Prefix tree clustering, template generalization |
| Fingerprint Cache | **WORKING** | SHA256-based exact duplicate detection with TTL |
| Context Window | **WORKING** | Preserves N lines around ERROR/FATAL logs |
| Memory Buffer | **WORKING** | Batch processing with configurable flush interval |
| Disk Buffer | **WORKING** | File-based persistence with eviction |
| Profile System | **WORKING** | Auto-detection, YAML parsing, signature verification |
| Stdout Output | **WORKING** | JSON-formatted log output |
| Loki Output | **WORKING** | v3+ support with structured metadata |
| Elasticsearch Output | **WORKING** | ECS-compliant bulk indexing |
| S3 Output | **STUB** | Logs intent but does not upload |
| CLI `logsieve server` | **PARTIAL** | Prints message, delegates to cmd/server |
| CLI `logsieve config example` | **WORKING** | Generates example config |
| CLI `logsieve config validate` | **WORKING** | Validates config files |
| CLI `logsieve version` | **WORKING** | Shows version info |
| CLI `logsieve capture` | **STUB** | Prints "not yet implemented" |
| CLI `logsieve learn` | **STUB** | Prints "not yet implemented" |
| CLI `logsieve audit` | **STUB** | Prints "not yet implemented" |
| Profile Hub Sync | **NOT WORKING** | Hub URL `hub.logsieve.io` does not exist |
| Helm Charts | **NOT PRESENT** | Referenced in Makefile but `helm/logsieve` does not exist |

### Core Functionality Assessment

```
Ingestion Pipeline:  [====================] 100% - Fully working
Deduplication:       [====================] 100% - Drain3 + Fingerprint
Profile System:      [=================== ] 95%  - Hub sync not functional
Output Routing:      [===============     ] 75%  - S3 is stub
CLI Tools:           [==========          ] 50%  - 3/6 commands are stubs
Infrastructure:      [============        ] 60%  - Missing Helm, load test fixtures
```

---

## 2. Stubbed Features Inventory

### CRITICAL: CLI Commands Not Implemented

#### Task 1: `logsieve capture` Command
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/cmd/logsieve/main.go:80-96`
- **Current State:** Prints stub message and returns nil
- **Target State:** Capture container logs via Docker API or kubectl
- **Acceptance Criteria:**
  - [ ] Connect to Docker daemon or kubectl
  - [ ] Stream logs from specified container
  - [ ] Write logs to specified output file
  - [ ] Support `--follow` for live streaming
  - [ ] Support `--duration` for time-limited capture
- **Estimated Complexity:** L (requires Docker/K8s integration)

#### Task 2: `logsieve learn` Command
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/cmd/logsieve/main.go:98-130`
- **Current State:** Prints stub message and returns nil
- **Target State:** Analyze log file and generate profile YAML
- **Acceptance Criteria:**
  - [ ] Read input log file
  - [ ] Run logs through Drain3 to discover templates
  - [ ] Generate fingerprint rules from top clusters
  - [ ] Output valid LogProfile YAML with metadata
  - [ ] Support `--coverage` to tune rule count
- **Estimated Complexity:** M (uses existing Drain3 code)

#### Task 3: `logsieve audit` Command
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/cmd/logsieve/main.go:132-158`
- **Current State:** Prints stub message and returns nil
- **Target State:** Evaluate profile effectiveness against logs
- **Acceptance Criteria:**
  - [ ] Load specified profile
  - [ ] Process input logs through profile rules
  - [ ] Report match/drop/template statistics
  - [ ] Calculate deduplication ratio
  - [ ] Support `--live` for real-time auditing
- **Estimated Complexity:** M (uses existing profile/dedup code)

### HIGH: S3 Output Adapter Not Implemented

#### Task 4: S3 Output Adapter
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/pkg/output/s3.go:26-49`
- **Current State:** Logs "S3 adapter not fully implemented" and serializes to buffer but never uploads
- **Target State:** Upload batched logs to S3 bucket
- **Acceptance Criteria:**
  - [ ] Initialize AWS S3 client with credentials
  - [ ] Batch logs into JSONL files
  - [ ] Upload with configurable prefix/key pattern
  - [ ] Support S3-compatible storage (MinIO)
  - [ ] Handle upload errors with retry
- **Estimated Complexity:** M (AWS SDK integration)

### MEDIUM: Profile Hub Integration

#### Task 5: Profile Hub Sync
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/pkg/profiles/manager.go:53-67`
- **Current State:** References `hub.logsieve.io` which does not exist
- **Target State:** Either implement hub server OR remove/mock for offline use
- **Acceptance Criteria:**
  - [ ] Option A: Remove hub references, work offline-only
  - [ ] Option B: Create simple hub API server
  - [ ] Option C: Point to GitHub raw URLs for community profiles
- **Estimated Complexity:** S (if offline-only) / L (if building hub)

### MEDIUM: Helm Charts Missing

#### Task 6: Create Helm Charts
- **File(s):** Makefile references `helm/logsieve` which does not exist
- **Current State:** `make helm-lint`, `helm-template`, `helm-package` fail
- **Target State:** Fully working Helm chart for Kubernetes deployment
- **Acceptance Criteria:**
  - [ ] Create `helm/logsieve/Chart.yaml`
  - [ ] Create `helm/logsieve/values.yaml`
  - [ ] Create deployment, service, configmap templates
  - [ ] Support HPA, PDB, ServiceMonitor
  - [ ] Pass `helm lint`
- **Estimated Complexity:** M

### LOW: CLI `server` Command Delegation

#### Task 7: CLI `server` Command
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/cmd/logsieve/main.go:47-66`
- **Current State:** Prints message suggesting to use `cmd/server` binary
- **Target State:** Actually start the server or clearly document the split
- **Acceptance Criteria:**
  - [ ] Option A: Embed server logic and run directly
  - [ ] Option B: Update message to be clearer and add `--help` guidance
- **Estimated Complexity:** S

### LOW: Missing Test Fixtures

#### Task 8: Integration Test Fixtures
- **File(s):**
  - `/Users/srijanshukla/code/projects/active-pending/logsieve/test/fixtures/` (empty)
  - `/Users/srijanshukla/code/projects/active-pending/logsieve/test/integration/` (empty)
- **Current State:** Directories exist but are empty
- **Target State:** Sample log files for testing
- **Acceptance Criteria:**
  - [ ] Add nginx access log samples
  - [ ] Add postgres log samples
  - [ ] Add java-spring log samples
  - [ ] Add integration test files
- **Estimated Complexity:** S

### LOW: Load Test Script Missing

#### Task 9: Load Test Lua Script
- **File(s):** Makefile references `test/load/basic.lua` which does not exist
- **Current State:** `make load-test` would fail
- **Target State:** Working wrk script for load testing
- **Acceptance Criteria:**
  - [ ] Create `test/load/basic.lua` for wrk
  - [ ] Generate realistic log payloads
  - [ ] Support configurable request rate
- **Estimated Complexity:** S

---

## 3. Priority Matrix

### MVP (Must Have for Production)

| Priority | Task | Complexity | Effort | Impact |
|----------|------|------------|--------|--------|
| P0 | S3 Output Adapter | M | 2-3 days | Enables archival workflows |
| P0 | Helm Charts | M | 2-3 days | Required for K8s deployment |
| P1 | Profile Hub (offline mode) | S | 1 day | Remove broken external dependency |
| P1 | Test Fixtures | S | 1 day | Enable proper testing |
| P1 | Load Test Script | S | 0.5 day | Enable performance validation |

### Nice to Have

| Priority | Task | Complexity | Effort | Impact |
|----------|------|------------|--------|--------|
| P2 | `logsieve learn` | M | 2-3 days | Enables profile generation |
| P2 | `logsieve audit` | M | 2 days | Enables profile evaluation |
| P3 | `logsieve capture` | L | 3-4 days | Enables log capture (Docker/K8s) |
| P3 | Fix CLI `server` command | S | 0.5 day | UX improvement |

---

## 4. Implementation Tasks

### Task: S3 Output Adapter
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/pkg/output/s3.go`
- **Current State:** Stub that serializes but does not upload
- **Target State:** Full S3 upload with batching
- **Implementation Notes:**
  ```go
  // Add to go.mod:
  // github.com/aws/aws-sdk-go-v2/...

  // Required config fields:
  // - bucket: string
  // - region: string
  // - prefix: string (key prefix pattern)
  // - credentials: AWS credential chain or explicit
  ```
- **Acceptance Criteria:**
  - [ ] `go test ./pkg/output/s3_test.go` passes
  - [ ] Can upload to real S3 bucket
  - [ ] Can upload to MinIO (S3-compatible)
  - [ ] Handles errors gracefully with retry
- **Estimated Complexity:** M

### Task: Helm Chart Creation
- **File(s):** Create `helm/logsieve/` directory structure
- **Current State:** Does not exist
- **Target State:** Production-ready Helm chart
- **Implementation Notes:**
  ```
  helm/logsieve/
  ├── Chart.yaml
  ├── values.yaml
  ├── templates/
  │   ├── deployment.yaml
  │   ├── service.yaml
  │   ├── configmap.yaml
  │   ├── hpa.yaml
  │   ├── pdb.yaml
  │   ├── serviceaccount.yaml
  │   └── servicemonitor.yaml
  └── README.md
  ```
- **Acceptance Criteria:**
  - [ ] `helm lint ./helm/logsieve` passes
  - [ ] `helm template logsieve ./helm/logsieve` generates valid YAML
  - [ ] Can deploy to Kubernetes cluster
- **Estimated Complexity:** M

### Task: Profile Hub Offline Mode
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/pkg/profiles/manager.go`
- **Current State:** References non-existent hub.logsieve.io
- **Target State:** Work in offline mode by default
- **Implementation Notes:**
  - Set default `trustMode: offline`
  - Remove or comment out hub sync logic
  - Document how to add profiles manually
- **Acceptance Criteria:**
  - [ ] Server starts without network errors
  - [ ] Profiles load from local path only
  - [ ] No references to external hub in startup logs
- **Estimated Complexity:** S

### Task: Create Test Fixtures
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/test/fixtures/`
- **Current State:** Empty directory
- **Target State:** Sample log files for all builtin profiles
- **Implementation Notes:**
  ```
  test/fixtures/
  ├── nginx-access.log       (100 sample lines)
  ├── nginx-error.log        (50 sample lines)
  ├── postgres.log           (100 sample lines)
  ├── java-spring.log        (100 sample lines)
  └── mixed.log              (mixed format for testing)
  ```
- **Acceptance Criteria:**
  - [ ] Files contain realistic log samples
  - [ ] Can be used for integration tests
  - [ ] Cover edge cases (multi-line, unicode, etc.)
- **Estimated Complexity:** S

### Task: Create Load Test Script
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/test/load/basic.lua`
- **Current State:** Does not exist
- **Target State:** Working wrk script for load testing
- **Implementation Notes:**
  ```lua
  -- Generate realistic log payloads
  -- Support configurable batch sizes
  -- Measure latency and throughput
  ```
- **Acceptance Criteria:**
  - [ ] `make load-test` runs without error
  - [ ] Reports requests/second
  - [ ] Reports latency percentiles
- **Estimated Complexity:** S

### Task: Implement `logsieve learn` Command
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/cmd/logsieve/main.go`
- **Current State:** Stub
- **Target State:** Generates profile from log file
- **Implementation Notes:**
  1. Read log file line by line
  2. Run each line through Drain3
  3. Extract top N clusters as fingerprint rules
  4. Generate valid YAML profile
- **Acceptance Criteria:**
  - [ ] Accepts `--input` and `--output` flags
  - [ ] Generates valid LogProfile YAML
  - [ ] Generated profile can be loaded by manager
- **Estimated Complexity:** M

### Task: Implement `logsieve audit` Command
- **File(s):** `/Users/srijanshukla/code/projects/active-pending/logsieve/cmd/logsieve/main.go`
- **Current State:** Stub
- **Target State:** Evaluates profile against logs
- **Implementation Notes:**
  1. Load specified profile
  2. Read input logs
  3. Apply profile rules
  4. Report statistics
- **Acceptance Criteria:**
  - [ ] Accepts `--profile` and `--input` flags
  - [ ] Reports match/drop/template counts
  - [ ] Reports deduplication ratio
- **Estimated Complexity:** M

---

## 5. Dependencies

### External Services Required

| Service | Purpose | Local Dev Alternative |
|---------|---------|----------------------|
| Loki | Log storage output | Docker: `grafana/loki:2.9.0` |
| Grafana | Log visualization | Docker: `grafana/grafana:10.1.0` |
| Elasticsearch | Log storage output | Docker: `elasticsearch:8.11.0` |
| S3 | Log archival | Docker: `minio/minio:latest` |
| Prometheus | Metrics scraping | Docker: `prom/prometheus:v2.47.0` |

### Local Development Stack

See `docker-compose.dev.yml` for a complete local development environment that includes:
- LogSieve server (built from source)
- Fluent Bit (log collector)
- Loki (log storage)
- Grafana (visualization)
- Prometheus (metrics)
- MinIO (S3-compatible storage)
- Log generator (test traffic)

---

## 6. Testing Checklist

### Unit Tests (Existing - ~85% coverage)

- [x] `internal/queue/queue_test.go`
- [x] `pkg/config/config_test.go`
- [x] `pkg/config/loader_test.go`
- [x] `pkg/ingestion/parser_test.go`
- [x] `pkg/ingestion/handler_test.go`
- [x] `pkg/ingestion/buffer_test.go`
- [x] `pkg/dedup/drain3_test.go`
- [x] `pkg/dedup/fingerprint_test.go`
- [x] `pkg/dedup/context_test.go`
- [x] `pkg/dedup/engine_test.go`
- [x] `pkg/output/router_test.go`
- [x] `pkg/output/stdout_test.go`
- [x] `pkg/output/loki_test.go`
- [x] `pkg/output/elasticsearch_test.go`
- [x] `pkg/output/s3_test.go`
- [x] `pkg/profiles/detector_test.go`
- [x] `pkg/profiles/parser_test.go`
- [x] `pkg/profiles/manager_test.go`
- [x] `pkg/processor/processor_test.go`
- [x] `pkg/metrics/prometheus_test.go`

### Integration Tests (Missing)

- [ ] `test/integration/ingestion_test.go` - End-to-end ingestion flow
- [ ] `test/integration/dedup_test.go` - Deduplication accuracy
- [ ] `test/integration/output_loki_test.go` - Real Loki integration
- [ ] `test/integration/output_es_test.go` - Real Elasticsearch integration
- [ ] `test/integration/profile_test.go` - Profile loading and matching

### Load Tests (Missing)

- [ ] `test/load/basic.lua` - Basic throughput test
- [ ] `test/load/spike.lua` - Spike traffic test
- [ ] `test/load/sustained.lua` - Sustained load test

### CLI Tests (Missing)

- [ ] `cmd/logsieve/main_test.go` - CLI command tests

---

## 7. Documentation Gaps

### README.md Issues

1. **Helm deployment section** references non-existent charts
2. **Profile hub** URL `hub.logsieve.io` does not exist
3. **License** section says "to be added"

### Missing Documentation

- [ ] `CONTRIBUTING.md` - How to contribute
- [ ] `CHANGELOG.md` - Version history
- [ ] `docs/ARCHITECTURE.md` - System design
- [ ] `docs/PROFILES.md` - Profile authoring guide
- [ ] `docs/DEPLOYMENT.md` - Production deployment guide

### Configuration Documentation

The `config.example.yaml` is good but missing:
- [ ] Comments explaining `trustMode` options
- [ ] Examples for S3 output configuration
- [ ] Examples for authentication headers

---

## 8. Quick Start Commands

After completing the MVP tasks, an engineer should be able to:

```bash
# Clone and build
git clone https://github.com/logsieve/logsieve.git
cd logsieve
make build

# Run tests
make test

# Start local dev environment
docker-compose -f docker-compose.dev.yml up

# Send test logs
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{"log": "User 123 logged in", "time": "2025-01-01T00:00:00Z"}'

# View metrics
curl http://localhost:9090/metrics | grep logsieve

# View logs in Grafana
open http://localhost:3000  # admin/admin
```

---

## 9. Recommended Implementation Order

1. **Week 1: Foundation**
   - [ ] Create `docker-compose.dev.yml` (DONE - see file)
   - [ ] Add test fixtures
   - [ ] Add load test script
   - [ ] Fix profile hub to work offline

2. **Week 2: Core Features**
   - [ ] Implement S3 output adapter
   - [ ] Create Helm charts

3. **Week 3: CLI Tools**
   - [ ] Implement `logsieve learn`
   - [ ] Implement `logsieve audit`

4. **Week 4: Polish**
   - [ ] Implement `logsieve capture`
   - [ ] Add integration tests
   - [ ] Update documentation

---

## 10. Success Criteria

The project is "production ready" when:

1. **All outputs work:** stdout, Loki, Elasticsearch, S3
2. **Local dev works:** `docker-compose.dev.yml up` runs full stack
3. **K8s deployment works:** Helm chart deploys successfully
4. **CLI is complete:** All commands implemented (or clearly documented as future)
5. **Tests pass:** `make test` succeeds with >80% coverage
6. **Documentation complete:** README reflects actual functionality
