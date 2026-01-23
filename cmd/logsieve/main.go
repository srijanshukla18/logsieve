package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/dedup"
	"github.com/logsieve/logsieve/pkg/profiles"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	cmd := newRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logsieve",
		Short: "LogSieve - Community-powered log deduplication and filtering",
		Long: `LogSieve is a high-performance log deduplication and filtering system that acts as a sidecar
between log collectors (like Fluent Bit) and storage backends (Loki, Elasticsearch, ClickHouse).
It reduces log volumes by ~90% using community-powered profiles.`,
	}

	cmd.AddCommand(
		newServerCommand(),
		newCaptureCommand(),
		newLearnCommand(),
		newAuditCommand(),
		newVersionCommand(),
		newConfigCommand(),
	)

	return cmd
}

func newServerCommand() *cobra.Command {
	var cfgFile string
	var logLevel string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the LogSieve HTTP server",
		Long:  "Start the LogSieve HTTP server for log ingestion, deduplication, and routing",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Server command - implementation delegated to cmd/server binary")
			fmt.Printf("Use: go run cmd/server/main.go --config=%s --log-level=%s\n", cfgFile, logLevel)
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	cmd.Flags().StringVarP(&logLevel, "log-level", "l", "", "log level (trace, debug, info, warn, error)")

	return cmd
}

func newCaptureCommand() *cobra.Command {
	var (
		container string
		duration  string
		output    string
		follow    bool
		source    string
	)

	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture logs from running containers",
		Long:  "Capture logs from running containers for profile creation and analysis",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

			var captureSource string
			if source == "auto" || source == "" {
				if isDockerAvailable() {
					captureSource = "docker"
				} else if isKubectlAvailable() {
					captureSource = "kubectl"
				} else {
					return fmt.Errorf("neither docker nor kubectl is available")
				}
			} else {
				captureSource = source
			}

			dur, err := time.ParseDuration(duration)
			if err != nil {
				return fmt.Errorf("invalid duration: %w", err)
			}

			var outFile *os.File
			if output != "" && output != "-" {
				outFile, err = os.Create(output)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer outFile.Close()
			} else {
				outFile = os.Stdout
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigCh
				logger.Info().Msg("Received interrupt signal, stopping capture")
				cancel()
			}()

			var captureCmd *exec.Cmd
			switch captureSource {
			case "docker":
				cmdArgs := []string{"logs"}
				if follow {
					cmdArgs = append(cmdArgs, "-f")
				}
				cmdArgs = append(cmdArgs, "--timestamps", container)
				captureCmd = exec.CommandContext(ctx, "docker", cmdArgs...)
			case "kubectl":
				cmdArgs := []string{"logs"}
				if follow {
					cmdArgs = append(cmdArgs, "-f")
				}
				cmdArgs = append(cmdArgs, "--timestamps", container)
				captureCmd = exec.CommandContext(ctx, "kubectl", cmdArgs...)
			default:
				return fmt.Errorf("unknown source: %s", captureSource)
			}

			captureCmd.Stderr = os.Stderr
			stdout, err := captureCmd.StdoutPipe()
			if err != nil {
				return fmt.Errorf("failed to get stdout pipe: %w", err)
			}

			if err := captureCmd.Start(); err != nil {
				return fmt.Errorf("failed to start capture: %w", err)
			}

			logger.Info().
				Str("container", container).
				Str("source", captureSource).
				Dur("duration", dur).
				Bool("follow", follow).
				Msg("Starting log capture")

			lineCount := 0
			scanner := bufio.NewScanner(stdout)

			if !follow {
				go func() {
					time.Sleep(dur)
					cancel()
				}()
			}

			doneCh := make(chan struct{})
			go func() {
				defer close(doneCh)
				for scanner.Scan() {
					line := scanner.Text()
					fmt.Fprintln(outFile, line)
					lineCount++
				}
			}()

			select {
			case <-ctx.Done():
				captureCmd.Process.Kill()
			case <-doneCh:
			}

			captureCmd.Wait()

			logger.Info().
				Int("lines", lineCount).
				Msg("Capture complete")

			if output != "" && output != "-" {
				fmt.Printf("Captured %d lines to %s\n", lineCount, output)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&container, "container", "", "Container name or ID to capture logs from")
	cmd.Flags().StringVar(&duration, "duration", "1h", "Duration to capture logs (e.g., 1h, 30m, 300s)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file for captured logs (default: stdout)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().StringVar(&source, "source", "auto", "Log source: docker, kubectl, or auto")

	cmd.MarkFlagRequired("container")

	return cmd
}

func newLearnCommand() *cobra.Command {
	var (
		input    string
		output   string
		coverage float64
		name     string
		image    string
	)

	cmd := &cobra.Command{
		Use:   "learn",
		Short: "Generate profile from captured logs",
		Long:  "Analyze captured logs and generate a LogSieve profile for deduplication",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

			file, err := os.Open(input)
			if err != nil {
				return fmt.Errorf("failed to open input file: %w", err)
			}
			defer file.Close()

			dedupCfg := config.DedupConfig{
				SimilarityThreshold: 0.4,
			}
			drain := dedup.NewDrain3(dedupCfg, logger)

			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

			lineCount := 0
			for scanner.Scan() {
				line := scanner.Text()
				if strings.TrimSpace(line) == "" {
					continue
				}
				drain.AddLogMessage(line)
				lineCount++
			}

			if err := scanner.Err(); err != nil {
				return fmt.Errorf("error reading input file: %w", err)
			}

			logger.Info().
				Int("lines", lineCount).
				Int("clusters", drain.GetPatternCount()).
				Msg("Analyzed log file")

			clusters := drain.GetClusters()
			totalMessages := 0
			for _, c := range clusters {
				totalMessages += c.Size
			}

			var fingerprintRules []profiles.FingerprintRule
			targetCoverage := int(float64(totalMessages) * coverage)
			coveredMessages := 0

			topClusters := drain.GetTopClusters(0)
			for _, cluster := range topClusters {
				if coveredMessages >= targetCoverage {
					break
				}

				pattern := templateToRegex(cluster.LogTemplate)
				fingerprintRules = append(fingerprintRules, profiles.FingerprintRule{
					Pattern: pattern,
					Action:  "template",
				})
				coveredMessages += cluster.Size
			}

			profileName := name
			if profileName == "" {
				profileName = strings.TrimSuffix(strings.TrimSuffix(input, ".log"), ".txt")
				if lastSlash := strings.LastIndex(profileName, "/"); lastSlash >= 0 {
					profileName = profileName[lastSlash+1:]
				}
			}

			imagePattern := image
			if imagePattern == "" {
				imagePattern = "*"
			}

			profile := profiles.Profile{
				APIVersion: "logsieve.io/v1",
				Kind:       "LogProfile",
				Metadata: profiles.ProfileMetadata{
					Name:        profileName,
					Version:     "1.0.0",
					Description: fmt.Sprintf("Auto-generated profile from %s (%d log lines)", input, lineCount),
					Tags:        []string{"auto-generated"},
					Images:      []string{imagePattern},
				},
				Spec: profiles.ProfileSpec{
					Fingerprints: fingerprintRules,
				},
			}

			yamlData, err := yaml.Marshal(&profile)
			if err != nil {
				return fmt.Errorf("failed to marshal profile: %w", err)
			}

			var outFile *os.File
			if output != "" && output != "-" {
				outFile, err = os.Create(output)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer outFile.Close()
			} else {
				outFile = os.Stdout
			}

			fmt.Fprintln(outFile, "---")
			fmt.Fprint(outFile, string(yamlData))

			actualCoverage := float64(coveredMessages) / float64(totalMessages)
			fmt.Fprintf(os.Stderr, "\nProfile generated:\n")
			fmt.Fprintf(os.Stderr, "  Lines analyzed: %d\n", lineCount)
			fmt.Fprintf(os.Stderr, "  Clusters found: %d\n", len(clusters))
			fmt.Fprintf(os.Stderr, "  Rules generated: %d\n", len(fingerprintRules))
			fmt.Fprintf(os.Stderr, "  Coverage: %.2f%%\n", actualCoverage*100)

			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "Input log file to analyze")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output profile file (default: stdout)")
	cmd.Flags().Float64Var(&coverage, "coverage", 0.95, "Target coverage ratio (0.0-1.0)")
	cmd.Flags().StringVar(&name, "name", "", "Profile name")
	cmd.Flags().StringVar(&image, "image", "", "Container image pattern")

	cmd.MarkFlagRequired("input")

	return cmd
}

func newAuditCommand() *cobra.Command {
	var (
		profilePath string
		live        bool
		input       string
		verbose     bool
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit profile effectiveness",
		Long:  "Analyze how well a profile performs against logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

			profileData, err := os.ReadFile(profilePath)
			if err != nil {
				return fmt.Errorf("failed to read profile: %w", err)
			}

			profile, err := profiles.ParseProfile(profileData)
			if err != nil {
				return fmt.Errorf("failed to parse profile: %w", err)
			}

			logger.Info().
				Str("profile", profile.Metadata.Name).
				Int("rules", len(profile.Spec.Fingerprints)).
				Msg("Loaded profile")

			var logReader *bufio.Scanner
			if input != "" {
				file, err := os.Open(input)
				if err != nil {
					return fmt.Errorf("failed to open input file: %w", err)
				}
				defer file.Close()
				logReader = bufio.NewScanner(file)
			} else if live {
				logReader = bufio.NewScanner(os.Stdin)
			} else {
				return fmt.Errorf("must specify --input or --live")
			}

			logReader.Buffer(make([]byte, 1024*1024), 1024*1024)

			stats := AuditStats{
				RuleMatches: make(map[int]int),
			}

			for logReader.Scan() {
				line := logReader.Text()
				if strings.TrimSpace(line) == "" {
					continue
				}

				stats.TotalLines++
				matched := false

				for i, rule := range profile.Spec.Fingerprints {
					if m, _ := rule.Matches(line); m {
						matched = true
						stats.RuleMatches[i]++

						switch rule.Action {
						case "drop":
							stats.DroppedLines++
						case "template":
							stats.TemplatedLines++
						case "keep":
							stats.KeptLines++
						}

						if verbose {
							fmt.Printf("[%s] Rule %d matched: %s\n", rule.Action, i, truncate(line, 80))
						}
						break
					}
				}

				if !matched {
					stats.UnmatchedLines++
					if verbose {
						fmt.Printf("[unmatched] %s\n", truncate(line, 80))
					}
				}
			}

			if err := logReader.Err(); err != nil {
				return fmt.Errorf("error reading logs: %w", err)
			}

			fmt.Println("\n=== Audit Results ===")
			fmt.Printf("Profile: %s (v%s)\n", profile.Metadata.Name, profile.Metadata.Version)
			fmt.Printf("Total lines: %d\n", stats.TotalLines)
			fmt.Printf("Matched lines: %d (%.2f%%)\n", stats.MatchedLines(), stats.MatchRate()*100)
			fmt.Printf("  - Dropped: %d\n", stats.DroppedLines)
			fmt.Printf("  - Templated: %d\n", stats.TemplatedLines)
			fmt.Printf("  - Kept: %d\n", stats.KeptLines)
			fmt.Printf("Unmatched lines: %d (%.2f%%)\n", stats.UnmatchedLines, (1-stats.MatchRate())*100)

			if stats.TotalLines > 0 {
				dedupRatio := float64(stats.DroppedLines+stats.TemplatedLines) / float64(stats.TotalLines)
				fmt.Printf("\nEstimated dedup ratio: %.2f%%\n", dedupRatio*100)
			}

			fmt.Println("\nRule effectiveness:")
			for i, rule := range profile.Spec.Fingerprints {
				count := stats.RuleMatches[i]
				pct := 0.0
				if stats.TotalLines > 0 {
					pct = float64(count) / float64(stats.TotalLines) * 100
				}
				fmt.Printf("  Rule %d [%s]: %d matches (%.2f%%)\n", i, rule.Action, count, pct)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "Profile to audit (path to YAML file)")
	cmd.Flags().BoolVar(&live, "live", false, "Audit against live logs from stdin")
	cmd.Flags().StringVar(&input, "input", "", "Input log file for audit")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show each matched/unmatched line")

	cmd.MarkFlagRequired("profile")

	return cmd
}

type AuditStats struct {
	TotalLines     int
	DroppedLines   int
	TemplatedLines int
	KeptLines      int
	UnmatchedLines int
	RuleMatches    map[int]int
}

func (s *AuditStats) MatchedLines() int {
	return s.DroppedLines + s.TemplatedLines + s.KeptLines
}

func (s *AuditStats) MatchRate() float64 {
	if s.TotalLines == 0 {
		return 0
	}
	return float64(s.MatchedLines()) / float64(s.TotalLines)
}

func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("LogSieve CLI\n")
			fmt.Printf("Version: %s\n", Version)
			fmt.Printf("Commit: %s\n", Commit)
			fmt.Printf("Build Time: %s\n", BuildTime)
		},
	}

	return cmd
}

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration management commands",
	}

	cmd.AddCommand(newConfigExampleCommand())
	cmd.AddCommand(newConfigValidateCommand())

	return cmd
}

func newConfigExampleCommand() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "example",
		Short: "Generate example configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				output = "config.example.yaml"
			}

			if err := config.WriteExample(output); err != nil {
				return fmt.Errorf("failed to write example config: %w", err)
			}

			fmt.Printf("Example configuration written to: %s\n", output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file for example config")

	return cmd
}

func newConfigValidateCommand() *cobra.Command {
	var cfgFile string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			fmt.Printf("Configuration is valid: %s\n", cfgFile)
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	cmd.MarkFlagRequired("config")

	return cmd
}

func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}

func isKubectlAvailable() bool {
	cmd := exec.Command("kubectl", "version", "--client")
	return cmd.Run() == nil
}

func templateToRegex(template []string) string {
	var parts []string
	for _, token := range template {
		if token == "<*>" || strings.HasPrefix(token, "<") && strings.HasSuffix(token, ">") {
			parts = append(parts, `\S+`)
		} else {
			parts = append(parts, escapeRegex(token))
		}
	}
	return "^" + strings.Join(parts, `\s+`) + "$"
}

func escapeRegex(s string) string {
	special := []string{"\\", ".", "+", "*", "?", "(", ")", "[", "]", "{", "}", "^", "$", "|"}
	result := s
	for _, char := range special {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
