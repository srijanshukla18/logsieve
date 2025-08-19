# LogSieve Project Overview

LogSieve is a high-performance log deduplication and filtering system that acts as a sidecar between log collectors (like Fluent Bit) and storage backends (Loki, Elasticsearch, ClickHouse). It reduces log volumes by ~90% using community-powered profiles.

## Core Value Proposition

- **Community-Powered**: Shared profiles for common stacks (nginx, postgres, java-spring, etc.)
- **Massive Reduction**: 90% log volume reduction with zero configuration
- **Fluent Bit Native**: Drop-in HTTP endpoint integration
- **Smart Deduplication**: Drain3 templating + fingerprinting + context preservation
- **Multi-Output**: Route to Loki, Elasticsearch, ClickHouse, S3, or stdout

## Architecture Components

### 1. Ingestion Layer (`pkg/ingestion/`)
- HTTP server accepting Fluent Bit JSON format
- Request parsing and validation
- Buffering and batching logic
- Profile and output parameter handling

### 2. Processing Pipeline (`pkg/processor/`)
- Orchestrates the entire log processing flow
- Manages deduplication, profile application, and output routing
- Handles batching and async processing

### 3. Deduplication Engine (`pkg/dedup/`)
- **Drain3**: Official spec-compliant implementation with proper clustering, template generalization, and parameter extraction
- **Fingerprinting**: SHA256-based exact duplicate detection
- **Context Windows**: Preserve N lines around errors/critical events
- **Advanced Features**: Configurable tree depth, masking rules, training/inference modes

### 4. Profile System (`pkg/profiles/`)
- **Auto-detection**: Match containers to profiles by image/log patterns
- **Community Profiles**: YAML-based rules for common applications
- **Processing Rules**: Drop, template, sample, transform, and route logs

### 5. Output System (`pkg/output/`)
- Multi-output routing with conditional logic
- **Loki**: Full v3+ support with structured metadata and cardinality management
- **Elasticsearch**: ECS-compliant document structure with proper field mapping
- **ClickHouse, S3**: Additional output adapters
- Retry logic and circuit breakers with detailed error reporting

### 6. Observability (`pkg/metrics/`)
- **Prometheus-compliant metrics** following official best practices
- Standard metrics: `build_info`, `start_time_seconds`, `uptime_seconds`
- Performance monitoring and cost savings tracking with proper naming conventions
- Profile effectiveness measurement with reduced cardinality

## Key Technologies

- **Go 1.21**: High-performance, concurrent processing
- **Gin**: HTTP framework for ingestion endpoints
- **Prometheus**: Metrics and observability
- **Viper**: Configuration management
- **Zerolog**: Structured logging
- **Cobra**: CLI framework

## Deployment Patterns

1. **Kubernetes Sidecar**: Alongside applications with Fluent Bit
2. **Standalone Service**: Centralized log processing
3. **Docker Compose**: Development and small deployments

## Performance Targets

- **Throughput**: 10,000 logs/second per instance
- **Latency**: <10ms p99 (improved with optimized Drain3 implementation)
- **Memory**: <100MB baseline, <500MB under load
- **CPU**: <0.5 cores baseline, <2 cores under load
- **Reduction**: >85% for typical workloads (enhanced with proper template generalization)
- **Clustering Accuracy**: >95% with official Drain3 algorithm