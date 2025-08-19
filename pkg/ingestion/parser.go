package ingestion

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type Parser struct {
	logger zerolog.Logger
}

func NewParser(logger zerolog.Logger) *Parser {
	return &Parser{
		logger: logger.With().Str("component", "parser").Logger(),
	}
}

func (p *Parser) Parse(req *IngestRequest) ([]LogEntry, error) {
	if len(req.Logs) > 0 {
		return req.Logs, nil
	}

	if req.Log != "" {
		return p.parseFluentBitSingle(req)
	}

	return nil, fmt.Errorf("no valid log format found")
}

func (p *Parser) parseFluentBitSingle(req *IngestRequest) ([]LogEntry, error) {
	entry := LogEntry{
		Message: req.Log,
		Source:  req.Source,
		Labels:  make(map[string]string),
	}

	if req.Labels != nil {
		entry.Labels = req.Labels
	}

	if req.Stream != "" {
		entry.Labels["stream"] = req.Stream
	}
	
	if req.Tag != "" {
		entry.Labels["tag"] = req.Tag
	}

	timestamp, err := p.parseTimestamp(req.Timestamp, req.Time)
	if err != nil {
		p.logger.Warn().Err(err).Msg("Failed to parse timestamp, using current time")
		timestamp = time.Now()
	}
	entry.Timestamp = timestamp

	p.extractMetadata(&entry)

	return []LogEntry{entry}, nil
}

func (p *Parser) parseTimestamp(timestamp, timeField string) (time.Time, error) {
	if timestamp != "" {
		return p.tryParseTime(timestamp)
	}
	if timeField != "" {
		return p.tryParseTime(timeField)
	}
	return time.Now(), nil
}

func (p *Parser) tryParseTime(timeStr string) (time.Time, error) {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05.999999Z07:00",
		"2006-01-02T15:04:05.999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05.999",
		"2006-01-02 15:04:05",
	}

	if unixTime, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
		if unixTime > 1e12 {
			return time.Unix(0, unixTime*int64(time.Nanosecond)), nil
		} else if unixTime > 1e9 {
			return time.Unix(0, unixTime*int64(time.Microsecond)), nil
		}
		return time.Unix(unixTime, 0), nil
	}

	if unixTime, err := strconv.ParseFloat(timeStr, 64); err == nil {
		sec := int64(unixTime)
		nsec := int64((unixTime - float64(sec)) * 1e9)
		return time.Unix(sec, nsec), nil
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", timeStr)
}

func (p *Parser) extractMetadata(entry *LogEntry) {
	if entry.Labels == nil {
		entry.Labels = make(map[string]string)
	}

	p.extractKubernetesMetadata(entry)
	p.extractLogLevel(entry)
	p.extractContainerInfo(entry)
}

func (p *Parser) extractKubernetesMetadata(entry *LogEntry) {
	if podName, exists := entry.Labels["io.kubernetes.pod.name"]; exists {
		entry.PodName = podName
		delete(entry.Labels, "io.kubernetes.pod.name")
	}
	
	if namespace, exists := entry.Labels["io.kubernetes.pod.namespace"]; exists {
		entry.Namespace = namespace
		delete(entry.Labels, "io.kubernetes.pod.namespace")
	}
	
	if containerName, exists := entry.Labels["io.kubernetes.container.name"]; exists {
		entry.ContainerName = containerName
		delete(entry.Labels, "io.kubernetes.container.name")
	}
	
	if nodeName, exists := entry.Labels["io.kubernetes.pod.node_name"]; exists {
		entry.NodeName = nodeName
		delete(entry.Labels, "io.kubernetes.pod.node_name")
	}
}

func (p *Parser) extractLogLevel(entry *LogEntry) {
	message := strings.ToUpper(entry.Message)
	
	logLevels := []string{"FATAL", "ERROR", "WARN", "WARNING", "INFO", "DEBUG", "TRACE"}
	
	for _, level := range logLevels {
		if strings.Contains(message, level) {
			if level == "WARNING" {
				entry.Level = "WARN"
			} else {
				entry.Level = level
			}
			break
		}
	}
	
	if entry.Level == "" && strings.Contains(strings.ToLower(message), "exception") {
		entry.Level = "ERROR"
	}
}

func (p *Parser) extractContainerInfo(entry *LogEntry) {
	if containerName, exists := entry.Labels["container_name"]; exists && entry.ContainerName == "" {
		entry.ContainerName = containerName
	}
	
	if containerID, exists := entry.Labels["container_id"]; exists {
		entry.ContainerID = containerID
		if len(containerID) > 12 {
			entry.ContainerID = containerID[:12]
		}
	}
}