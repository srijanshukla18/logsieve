# Profile Development Guide

## Profile Structure and Schema

### Complete Profile Schema
```yaml
apiVersion: hub.logsieve.io/v1
kind: LogProfile
metadata:
  name: component-name           # Unique identifier
  version: "1.0.0"              # Semantic version
  author: "@username"           # GitHub username
  description: "Brief description of what this profile handles"
  tags: ["tag1", "tag2"]        # Searchable tags
  images:                       # Container images this profile applies to
    - "nginx:*"
    - "bitnami/nginx:*"

spec:
  fingerprints:                 # Pattern matching and action rules
    - pattern: "regex_pattern"
      action: "drop|template|keep"
      preserve: ["field1", "field2"]  # Fields to preserve in templates
      unless: "condition"              # Exception condition
  
  contextTriggers:              # When to preserve surrounding context
    - pattern: "ERROR|FATAL"
      before: 5                 # Lines before trigger
      after: 10                 # Lines after trigger
  
  sampling:                     # Probabilistic sampling rules
    - pattern: "GET /health"
      rate: 0.01               # Keep 1% of matches
  
  transforms:                   # Data transformation rules
    - field: "message"
      regex: "(password=)[^&\\s]+"
      replace: "$1***"
  
  routing:                      # Output routing rules
    rules:
      - name: "errors"
        pattern: "ERROR|FATAL"
        output: "loki-errors"
```

## Profile Development Workflow

### 1. Log Analysis Phase
```bash
# Capture sample logs from target application
logsieve capture --container nginx --duration 1h --output nginx-sample.log

# Analyze log patterns
grep -E "^[0-9]" nginx-sample.log | head -100
grep -E "ERROR|WARN" nginx-sample.log | head -50
```

### 2. Pattern Identification
Look for these common patterns:
- **High-volume, low-value**: Health checks, heartbeats, routine operations
- **Template-able**: Logs with variable data that can be parameterized
- **Critical events**: Errors, warnings, security events
- **Sensitive data**: Passwords, tokens, PII that needs scrubbing

### 3. Profile Creation
Start with the most specific patterns first:

```yaml
spec:
  fingerprints:
    # Most specific patterns first
    - pattern: 'FATAL: database connection failed'
      action: "keep"
    
    # More general patterns
    - pattern: 'ERROR:'
      action: "template"
    
    # High-volume drops last
    - pattern: 'GET /health'
      action: "drop"
```

## Pattern Writing Best Practices

### Regex Patterns
1. **Escape special characters**: `\.`, `\[`, `\]`, `\(`, `\)`, `\+`, `\*`, `\?`, `\^`, `\$`, `\|`
2. **Use character classes**: `\d+` for numbers, `\w+` for words
3. **Anchor when needed**: `^` for line start, `$` for line end
4. **Non-greedy matching**: `.*?` instead of `.*`

### Common Pattern Examples
```yaml
# IP addresses
- pattern: '\d+\.\d+\.\d+\.\d+'

# HTTP requests
- pattern: '"[A-Z]+ /[^"]*" \d+ \d+'

# Timestamps
- pattern: '\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}'

# Database queries
- pattern: 'SELECT .* FROM \w+'

# Error levels
- pattern: '(ERROR|FATAL|PANIC):'
```

## Action Types and Usage

### Drop Action
Use for high-volume, low-value logs:
```yaml
- pattern: 'GET /health'
  action: "drop"

- pattern: 'LOG: checkpoint starting'
  action: "drop"
```

### Template Action
Use for logs with variable data:
```yaml
- pattern: 'User \d+ logged in from \d+\.\d+\.\d+\.\d+'
  action: "template"
  preserve: ["user_id", "ip_address"]
```

### Keep Action
Use for critical events that should never be dropped:
```yaml
- pattern: 'FATAL:|PANIC:|SECURITY:'
  action: "keep"
```

## Context Preservation

### Error Context
Preserve context around errors for debugging:
```yaml
contextTriggers:
  - pattern: 'ERROR:|FATAL:|PANIC:'
    before: 5    # 5 lines before error
    after: 10    # 10 lines after error
  
  - pattern: 'Exception in thread'
    before: 2
    after: 20    # Capture full stack trace
```

### Performance Context
Preserve context around slow operations:
```yaml
contextTriggers:
  - pattern: 'duration: [0-9]{4,}'  # >1000ms
    before: 2
    after: 3
```

## Sampling Strategies

### Health Check Sampling
```yaml
sampling:
  - pattern: 'GET /health'
    rate: 0.01    # Keep 1%
  
  - pattern: 'GET /ready'
    rate: 0.01    # Keep 1%
```

### Success Rate Sampling
```yaml
sampling:
  - pattern: '200 \d+'     # HTTP 200 responses
    rate: 0.1              # Keep 10%
  
  - pattern: '404 \d+'     # HTTP 404 responses  
    rate: 0.5              # Keep 50%
```

### Database Connection Sampling
```yaml
sampling:
  - pattern: 'connection received:'
    rate: 0.01             # Keep 1%
  
  - pattern: 'connection authorized:'
    rate: 0.01             # Keep 1%
```

## Data Transformation

### Sensitive Data Scrubbing
```yaml
transforms:
  # Passwords
  - field: "message"
    regex: "(password\\s*=\\s*')[^']*'"
    replace: "$1***'"
  
  # API tokens
  - field: "message"
    regex: "(token=)[^&\\s]+"
    replace: "$1***"
  
  # Credit card numbers
  - field: "message"
    regex: "\\b\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}\\b"
    replace: "****-****-****-****"
  
  # SQL VALUES clauses
  - field: "message"
    regex: "(VALUES\\s*\\()[^)]*(\\))"
    replace: "$1***$2"
```

### Field Normalization
```yaml
transforms:
  # Normalize log levels
  - field: "level"
    regex: "WARN"
    replace: "WARNING"
  
  # Extract structured data
  - field: "message"
    regex: "user_id=(\\d+)"
    replace: "user_id=$1"
    extract_to: "user_id"
```

## Output Routing

### Error Routing
```yaml
routing:
  rules:
    - name: "critical_errors"
      pattern: "FATAL:|PANIC:|CRITICAL:"
      output: "pagerduty-alerts"
    
    - name: "application_errors"
      pattern: "ERROR:"
      output: "loki-errors"
    
    - name: "security_events"
      pattern: "SECURITY:|UNAUTHORIZED|FORBIDDEN"
      output: "security-siem"
```

### Performance Routing
```yaml
routing:
  rules:
    - name: "slow_queries"
      pattern: "duration: [0-9]{4,}"
      output: "performance-monitoring"
    
    - name: "access_logs"
      pattern: '\\d+\\.\\d+\\.\\d+\\.\\d+ - -'
      output: "loki-access"
```

## Profile Testing

### Coverage Testing
Ensure your profile covers >95% of log patterns:
```bash
# Test profile against sample logs
logsieve audit --profile nginx.yaml --input nginx-sample.log

# Expected output:
# ✅ Profile nginx: 98.2% coverage
# ⚠️ 12 new patterns detected (saved to /tmp/unknown.log)
```

### Performance Testing
Test profile performance with realistic load:
```bash
# Load test with profile
logsieve test-profile --profile nginx.yaml --rate 1000 --duration 60s
```

## Common Application Profiles

### Web Servers (Nginx, Apache)
Focus on:
- Access log templating
- Error log preservation
- Health check dropping
- Static asset sampling

### Databases (PostgreSQL, MySQL)
Focus on:
- Connection log sampling
- Query log templating
- Error preservation with context
- Checkpoint/maintenance log dropping

### Application Servers (Java, Node.js)
Focus on:
- Stack trace context preservation
- Request/response templating
- Health check dropping
- Performance metric extraction

### Message Queues (Redis, RabbitMQ)
Focus on:
- Connection event sampling
- Message processing templating
- Error preservation
- Heartbeat dropping

## Profile Validation Checklist

- [ ] **Metadata complete**: name, version, author, description, tags, images
- [ ] **Pattern coverage**: >95% of common log patterns covered
- [ ] **Performance tested**: No significant latency impact
- [ ] **Security reviewed**: All sensitive data properly scrubbed
- [ ] **Context preserved**: Critical events have appropriate context
- [ ] **Sampling tuned**: High-volume logs appropriately sampled
- [ ] **Routing configured**: Logs routed to appropriate outputs
- [ ] **Documentation**: Clear description and usage examples