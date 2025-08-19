package profiles

import (
	"strings"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/ingestion"
)

type Detector struct {
	logger zerolog.Logger
	rules  []DetectionRule
}

type DetectionRule struct {
	ProfileName   string
	ImagePatterns []string
	LogPatterns   []string
	Priority      int
}

func NewDetector(logger zerolog.Logger) *Detector {
	return &Detector{
		logger: logger.With().Str("component", "detector").Logger(),
		rules:  defaultDetectionRules(),
	}
}

func defaultDetectionRules() []DetectionRule {
	return []DetectionRule{
		{
			ProfileName: "nginx",
			ImagePatterns: []string{
				"nginx",
				"nginx:",
				"bitnami/nginx",
			},
			LogPatterns: []string{
				`\d+\.\d+\.\d+\.\d+ - -`,
				`"GET `,
				`"POST `,
				`HTTP/1.1"`,
			},
			Priority: 10,
		},
		{
			ProfileName: "postgres",
			ImagePatterns: []string{
				"postgres",
				"postgres:",
				"postgresql",
				"bitnami/postgresql",
			},
			LogPatterns: []string{
				"LOG:",
				"ERROR:",
				"FATAL:",
				"database system",
				"connection received",
			},
			Priority: 10,
		},
		{
			ProfileName: "java-spring",
			ImagePatterns: []string{
				"openjdk",
				"java:",
				"spring",
			},
			LogPatterns: []string{
				"org.springframework",
				"java.lang.",
				"ERROR",
				"INFO",
				"DEBUG",
				"SpringApplication",
			},
			Priority: 8,
		},
		{
			ProfileName: "redis",
			ImagePatterns: []string{
				"redis",
				"redis:",
				"bitnami/redis",
			},
			LogPatterns: []string{
				"Redis server",
				"Ready to accept connections",
				"DB loaded from disk",
			},
			Priority: 10,
		},
		{
			ProfileName: "mysql",
			ImagePatterns: []string{
				"mysql",
				"mysql:",
				"mariadb",
				"percona",
			},
			LogPatterns: []string{
				"mysqld:",
				"InnoDB:",
				"Query_time:",
				"ready for connections",
			},
			Priority: 10,
		},
	}
}

func (d *Detector) Detect(entry *ingestion.LogEntry) string {
	bestMatch := ""
	bestScore := 0

	for _, rule := range d.rules {
		score := d.calculateScore(entry, rule)
		if score > bestScore {
			bestScore = score
			bestMatch = rule.ProfileName
		}
	}

	if bestScore > 0 {
		d.logger.Debug().
			Str("profile", bestMatch).
			Int("score", bestScore).
			Str("container", entry.ContainerName).
			Msg("Detected profile")
		return bestMatch
	}

	return ""
}

func (d *Detector) calculateScore(entry *ingestion.LogEntry, rule DetectionRule) int {
	score := 0

	score += d.checkImagePatterns(entry, rule.ImagePatterns)
	score += d.checkLogPatterns(entry, rule.LogPatterns)

	return score * rule.Priority
}

func (d *Detector) checkImagePatterns(entry *ingestion.LogEntry, patterns []string) int {
	containerName := strings.ToLower(entry.ContainerName)
	
	if entry.Labels != nil {
		if image, ok := entry.Labels["image"]; ok {
			containerName = strings.ToLower(image)
		}
		if image, ok := entry.Labels["container_image"]; ok {
			containerName = strings.ToLower(image)
		}
	}

	if containerName == "" {
		return 0
	}

	for _, pattern := range patterns {
		if strings.Contains(containerName, strings.ToLower(pattern)) {
			return 10
		}
	}

	return 0
}

func (d *Detector) checkLogPatterns(entry *ingestion.LogEntry, patterns []string) int {
	message := entry.Message
	matchCount := 0

	for _, pattern := range patterns {
		if strings.Contains(message, pattern) {
			matchCount++
		}
	}

	if matchCount == 0 {
		return 0
	}

	return matchCount * 2
}

func (d *Detector) AddRule(rule DetectionRule) {
	d.rules = append(d.rules, rule)
}

func (d *Detector) GetRules() []DetectionRule {
	return d.rules
}