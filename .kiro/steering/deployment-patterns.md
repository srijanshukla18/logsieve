# LogSieve Deployment Patterns

## Deployment Architecture Overview

LogSieve supports multiple deployment patterns to fit different infrastructure needs:

1. **Kubernetes Sidecar**: Co-located with applications
2. **Standalone Service**: Centralized log processing
3. **Docker Compose**: Development and small deployments
4. **Helm Chart**: Production Kubernetes deployments

## Kubernetes Sidecar Pattern

### Sidecar Injection
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app-with-logsieve
  annotations:
    logsieve.io/inject: "true"
    logsieve.io/profile: "auto"
spec:
  containers:
  - name: app
    image: myapp:latest
    ports:
    - containerPort: 8080
  
  - name: fluent-bit
    image: fluent/fluent-bit:2.1
    volumeMounts:
    - name: fluent-bit-config
      mountPath: /fluent-bit/etc
    - name: varlog
      mountPath: /var/log
      readOnly: true
  
  - name: logsieve
    image: logsieve/sieve:latest
    ports:
    - containerPort: 8080
      name: http
    - containerPort: 9090
      name: metrics
    env:
    - name: LOGSIEVE_PROFILES_AUTODETECT
      value: "true"
    - name: LOGSIEVE_OUTPUTS_0_TYPE
      value: "loki"
    - name: LOGSIEVE_OUTPUTS_0_URL
      value: "http://loki:3100"
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 256Mi
  
  volumes:
  - name: fluent-bit-config
    configMap:
      name: fluent-bit-config
  - name: varlog
    hostPath:
      path: /var/log
```

### Fluent Bit Configuration
```ini
[SERVICE]
    Flush         5
    Daemon        off
    Log_Level     info
    HTTP_Server   On
    HTTP_Listen   0.0.0.0
    HTTP_Port     2020

[INPUT]
    Name              tail
    Path              /var/log/containers/*.log
    Parser            cri
    Tag               kube.*
    Refresh_Interval  5
    Mem_Buf_Limit     50MB
    Skip_Long_Lines   On

[OUTPUT]
    Name          http
    Match         *
    Host          localhost
    Port          8080
    URI           /ingest?profile=auto
    Format        json
    Retry_Limit   3
    
    # Headers
    Header        X-Source fluent-bit
    Header        Content-Type application/json
```

## Standalone Service Pattern

### Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: logsieve
  labels:
    app: logsieve
spec:
  replicas: 3
  selector:
    matchLabels:
      app: logsieve
  template:
    metadata:
      labels:
        app: logsieve
    spec:
      containers:
      - name: logsieve
        image: logsieve/sieve:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: LOGSIEVE_SERVER_PORT
          value: "8080"
        - name: LOGSIEVE_METRICS_PORT
          value: "9090"
        - name: LOGSIEVE_PROFILES_HUBURL
          value: "https://hub.logsieve.io"
        - name: LOGSIEVE_PROFILES_SYNCINTERVAL
          value: "1h"
        volumeMounts:
        - name: config
          mountPath: /etc/logsieve
        - name: profiles
          mountPath: /var/lib/logsieve/profiles
        resources:
          requests:
            cpu: 200m
            memory: 256Mi
          limits:
            cpu: 1000m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: logsieve-config
      - name: profiles
        persistentVolumeClaim:
          claimName: logsieve-profiles

---
apiVersion: v1
kind: Service
metadata:
  name: logsieve
  labels:
    app: logsieve
spec:
  selector:
    app: logsieve
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
  type: ClusterIP

---
apiVersion: v1
kind: Service
metadata:
  name: logsieve-lb
  labels:
    app: logsieve
spec:
  selector:
    app: logsieve
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  type: LoadBalancer
```

### ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: logsieve-config
data:
  config.yaml: |
    server:
      port: 8080
      address: "0.0.0.0"
      readTimeout: 30s
      writeTimeout: 30s

    ingestion:
      maxBatchSize: 1000
      flushInterval: 5s
      maxMemoryMB: 200
      bufferSize: 20000

    dedup:
      engine: "drain3"
      cacheSize: 20000
      contextLines: 5
      similarityThreshold: 0.9

    profiles:
      autoDetect: true
      hubURL: "https://hub.logsieve.io"
      syncInterval: 1h
      localPath: "/var/lib/logsieve/profiles"

    outputs:
      - name: "loki"
        type: "loki"
        url: "http://loki:3100"
        batchSize: 100
        timeout: 10s
        retries: 3

    metrics:
      enabled: true
      port: 9090
      path: "/metrics"

    logging:
      level: "info"
      structured: true
```

## Helm Chart Deployment

### Values Configuration
```yaml
# values.yaml
replicaCount: 3

image:
  repository: logsieve/sieve
  tag: "latest"
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080
  metricsPort: 9090

ingress:
  enabled: true
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
  hosts:
    - host: logsieve.example.com
      paths:
        - path: /
          pathType: Prefix

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
  targetMemoryUtilizationPercentage: 80

config:
  server:
    port: 8080
    readTimeout: 30s
    writeTimeout: 30s
  
  ingestion:
    maxBatchSize: 1000
    flushInterval: 5s
    maxMemoryMB: 200
  
  dedup:
    engine: "drain3"
    cacheSize: 20000
    contextLines: 5
  
  profiles:
    autoDetect: true
    hubURL: "https://hub.logsieve.io"
    syncInterval: 1h
  
  outputs:
    - name: "loki"
      type: "loki"
      url: "http://loki-gateway"
      batchSize: 100

monitoring:
  serviceMonitor:
    enabled: true
    interval: 30s
    path: /metrics
  
  grafanaDashboard:
    enabled: true

persistence:
  enabled: true
  storageClass: "fast-ssd"
  size: 10Gi
  accessMode: ReadWriteOnce

fluentbit:
  enabled: true
  config:
    outputs:
      - name: http
        host: logsieve
        port: 8080
        uri: /ingest?profile=auto
```

### Helm Installation
```bash
# Add LogSieve Helm repository
helm repo add logsieve https://logsieve.github.io/charts
helm repo update

# Install with default values
helm install my-logsieve logsieve/logsieve

# Install with custom values
helm install my-logsieve logsieve/logsieve \
  --values values.yaml \
  --set replicaCount=5 \
  --set config.outputs[0].url=http://my-loki:3100

# Install with Fluent Bit sidecar injection
helm install my-logsieve logsieve/logsieve \
  --set fluentbit.enabled=true \
  --set fluentbit.sidecarInjection.enabled=true
```

## Docker Compose Pattern

### Development Setup
```yaml
# docker-compose.yml
version: '3.8'

services:
  app:
    image: nginx:latest
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    depends_on:
      - logsieve

  fluent-bit:
    image: fluent/fluent-bit:2.1
    volumes:
      - ./fluent-bit.conf:/fluent-bit/etc/fluent-bit.conf
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
      - /var/log:/var/log:ro
    depends_on:
      - logsieve

  logsieve:
    image: logsieve/sieve:latest
    ports:
      - "8080:8080"
      - "9090:9090"
    environment:
      - LOGSIEVE_PROFILES_AUTODETECT=true
      - LOGSIEVE_OUTPUTS_0_TYPE=loki
      - LOGSIEVE_OUTPUTS_0_URL=http://loki:3100
    volumes:
      - ./config.yaml:/etc/logsieve/config.yaml
      - ./profiles:/etc/logsieve/profiles
    depends_on:
      - loki

  loki:
    image: grafana/loki:latest
    ports:
      - "3100:3100"
    command: -config.file=/etc/loki/local-config.yaml
    volumes:
      - loki-data:/loki

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana
      - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
      - ./grafana/datasources:/etc/grafana/provisioning/datasources

volumes:
  loki-data:
  grafana-data:
```

### Production Docker Compose
```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  logsieve:
    image: logsieve/sieve:${LOGSIEVE_VERSION:-latest}
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.2'
          memory: 256M
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
    ports:
      - "8080:8080"
      - "9090:9090"
    environment:
      - LOGSIEVE_LOGGING_LEVEL=info
      - LOGSIEVE_METRICS_ENABLED=true
      - LOGSIEVE_PROFILES_HUBURL=https://hub.logsieve.io
    volumes:
      - ./config/production.yaml:/etc/logsieve/config.yaml:ro
      - profiles-data:/var/lib/logsieve/profiles
      - ./ssl:/etc/ssl/logsieve:ro
    networks:
      - logsieve-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

  nginx:
    image: nginx:alpine
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/ssl/nginx:ro
    depends_on:
      - logsieve
    networks:
      - logsieve-network

volumes:
  profiles-data:

networks:
  logsieve-network:
    driver: overlay
    attachable: true
```

## Configuration Management

### Environment-Specific Configs
```yaml
# config/development.yaml
server:
  port: 8080
  address: "0.0.0.0"

logging:
  level: "debug"
  structured: false

profiles:
  localPath: "./profiles"
  hubURL: "http://localhost:3001"  # Local hub for testing

outputs:
  - name: "stdout"
    type: "stdout"

---
# config/staging.yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s

logging:
  level: "info"
  structured: true

profiles:
  hubURL: "https://staging-hub.logsieve.io"
  syncInterval: 30m

outputs:
  - name: "loki-staging"
    type: "loki"
    url: "http://loki-staging:3100"

---
# config/production.yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
  idleTimeout: 60s

ingestion:
  maxBatchSize: 2000
  maxMemoryMB: 500
  bufferSize: 50000

dedup:
  cacheSize: 50000
  contextLines: 10

profiles:
  hubURL: "https://hub.logsieve.io"
  syncInterval: 1h

outputs:
  - name: "loki-prod"
    type: "loki"
    url: "http://loki-gateway:3100"
    batchSize: 200
    retries: 5

logging:
  level: "warn"
  structured: true

metrics:
  enabled: true
  port: 9090
```

## Security Considerations

### RBAC Configuration
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: logsieve
  namespace: logging

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: logsieve
rules:
- apiGroups: [""]
  resources: ["pods", "namespaces"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: logsieve
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: logsieve
subjects:
- kind: ServiceAccount
  name: logsieve
  namespace: logging
```

### Network Policies
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: logsieve-network-policy
spec:
  podSelector:
    matchLabels:
      app: logsieve
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: fluent-bit
    ports:
    - protocol: TCP
      port: 8080
  - from:
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: loki
    ports:
    - protocol: TCP
      port: 3100
  - to: []
    ports:
    - protocol: TCP
      port: 443  # HTTPS for hub access
```

## Monitoring and Observability

### ServiceMonitor for Prometheus
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: logsieve
  labels:
    app: logsieve
spec:
  selector:
    matchLabels:
      app: logsieve
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
    honorLabels: true
```

### Grafana Dashboard ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: logsieve-dashboard
  labels:
    grafana_dashboard: "1"
data:
  logsieve-overview.json: |
    {
      "dashboard": {
        "title": "LogSieve Overview",
        "panels": [
          {
            "title": "Log Ingestion Rate",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(logsieve_ingestion_logs_total[5m])",
                "legendFormat": "{{source}}"
              }
            ]
          },
          {
            "title": "Deduplication Ratio",
            "type": "stat",
            "targets": [
              {
                "expr": "logsieve_dedup_ratio",
                "legendFormat": "{{profile}}"
              }
            ]
          }
        ]
      }
    }
```

## Scaling Strategies

### Horizontal Pod Autoscaler
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: logsieve-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: logsieve
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  - type: Pods
    pods:
      metric:
        name: logsieve_ingestion_logs_per_second
      target:
        type: AverageValue
        averageValue: "1000"
```

### Vertical Pod Autoscaler
```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: logsieve-vpa
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: logsieve
  updatePolicy:
    updateMode: "Auto"
  resourcePolicy:
    containerPolicies:
    - containerName: logsieve
      maxAllowed:
        cpu: 2000m
        memory: 2Gi
      minAllowed:
        cpu: 100m
        memory: 128Mi
```