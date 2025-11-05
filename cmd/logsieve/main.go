package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/dedup"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
	"github.com/logsieve/logsieve/pkg/profiles"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var (
	cfgFile     string
	verbose     bool
	inputFile   string
	outputFile  string
	profileName string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "logsieve",
		Short: "LogSieve CLI - Log deduplication and filtering tool",
		Long: `LogSieve is a high-performance log deduplication and filtering system
that reduces log volumes by ~90% using community-powered profiles.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildTime),
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(captureCmd())
	rootCmd.AddCommand(learnCmd())
	rootCmd.AddCommand(analyzeCmd())
	rootCmd.AddCommand(profilesCmd())
	rootCmd.AddCommand(statsCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("LogSieve CLI\n")
			fmt.Printf("Version:    %s\n", Version)
			fmt.Printf("Commit:     %s\n", Commit)
			fmt.Printf("Build Time: %s\n", BuildTime)
		},
	}
}

func captureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture logs from stdin and save to file",
		Long: `Capture log messages from stdin and save them to a file.
Useful for creating sample logs for profile development.`,
		Example: `  # Capture logs from docker container
  docker logs myapp | logsieve capture --output sample.log

  # Capture logs from kubectl
  kubectl logs pod/myapp -f | logsieve capture -o sample.log`,
		Run: func(cmd *cobra.Command, args []string) {
			if outputFile == "" {
				fmt.Fprintln(os.Stderr, "Error: output file is required")
				os.Exit(1)
			}

			if err := captureLogs(outputFile); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "output file path (required)")
	cmd.MarkFlagRequired("output")

	return cmd
}

func learnCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "learn",
		Short: "Learn patterns from log file and generate profile",
		Long: `Analyze a log file using Drain3 algorithm and generate a profile YAML.
This helps create custom profiles for your applications.`,
		Example: `  # Learn from log file
  logsieve learn -i sample.log -o myapp.yaml

  # Learn with specific similarity threshold
  logsieve learn -i sample.log -o myapp.yaml --threshold 0.6`,
		Run: func(cmd *cobra.Command, args []string) {
			if inputFile == "" || outputFile == "" {
				fmt.Fprintln(os.Stderr, "Error: both input and output files are required")
				os.Exit(1)
			}

			if err := learnPatterns(inputFile, outputFile); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&inputFile, "input", "i", "", "input log file (required)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "output profile file (required)")
	cmd.MarkFlagRequired("input")
	cmd.MarkFlagRequired("output")

	return cmd
}

func analyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze log file and show statistics",
		Long:  `Analyze a log file and display statistics about patterns, deduplication, etc.`,
		Example: `  # Analyze log file
  logsieve analyze -i sample.log

  # Analyze with specific profile
  logsieve analyze -i sample.log --profile nginx`,
		Run: func(cmd *cobra.Command, args []string) {
			if inputFile == "" {
				fmt.Fprintln(os.Stderr, "Error: input file is required")
				os.Exit(1)
			}

			if err := analyzeLogs(inputFile, profileName); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&inputFile, "input", "i", "", "input log file (required)")
	cmd.Flags().StringVar(&profileName, "profile", "", "profile to use for analysis")
	cmd.MarkFlagRequired("input")

	return cmd
}

func profilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "Manage log profiles",
		Long:  `List, validate, and manage log processing profiles.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available profiles",
		Run: func(cmd *cobra.Command, args []string) {
			if err := listProfiles(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "validate [profile.yaml]",
		Short: "Validate a profile file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateProfile(args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Profile %s is valid\n", args[0])
		},
	})

	return cmd
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats [server-url]",
		Short: "Get statistics from running LogSieve server",
		Long:  `Query a running LogSieve server for current statistics.`,
		Example: `  # Get stats from local server
  logsieve stats http://localhost:8080

  # Get stats from remote server
  logsieve stats https://logsieve.example.com`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			url := args[0]
			if !strings.HasPrefix(url, "http") {
				url = "http://" + url
			}

			fmt.Printf("Querying stats from: %s\n", url)
			fmt.Println("(This command requires implementation of HTTP client)")
		},
	}
}

// Capture logs from stdin
func captureLogs(outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	scanner := bufio.NewScanner(os.Stdin)
	count := 0

	fmt.Println("Capturing logs... (Press Ctrl+C to stop)")

	for scanner.Scan() {
		line := scanner.Text()
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write line: %w", err)
		}
		count++

		if count%1000 == 0 {
			fmt.Printf("\rCaptured %d lines...", count)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	fmt.Printf("\nCapture complete. Wrote %d lines to %s\n", count, outputPath)
	return nil
}

// Learn patterns from log file using Drain3
func learnPatterns(inputPath, outputPath string) error {
	fmt.Printf("Learning patterns from: %s\n", inputPath)

	// Setup logger
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	// Create Drain3 engine
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}

	metricsRegistry := metrics.NewRegistry()
	engine := dedup.NewEngine(cfg, metricsRegistry, logger)

	// Read log file
	file, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		entry := &ingestion.LogEntry{
			Message:   line,
			Timestamp: time.Now(),
		}

		if _, err := engine.Process(entry); err != nil {
			logger.Warn().Err(err).Msg("Failed to process line")
		}

		lineCount++
		if lineCount%1000 == 0 {
			fmt.Printf("\rProcessed %d lines...", lineCount)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	fmt.Printf("\nProcessed %d lines\n", lineCount)

	// Get statistics
	stats := engine.GetStats()
	fmt.Printf("Discovered %d unique patterns\n", stats.PatternCount)

	// Generate profile (simplified version)
	profile := map[string]interface{}{
		"apiVersion": "hub.logsieve.io/v1",
		"kind":       "LogProfile",
		"metadata": map[string]interface{}{
			"name":        "generated",
			"version":     "1.0.0",
			"description": "Auto-generated profile",
			"tags":        []string{"generated"},
			"generated":   time.Now().Format(time.RFC3339),
		},
		"spec": map[string]interface{}{
			"fingerprints": []interface{}{},
		},
		"stats": map[string]interface{}{
			"total_lines":    lineCount,
			"pattern_count":  stats.PatternCount,
			"learning_date":  time.Now().Format(time.RFC3339),
		},
	}

	// Write profile to file
	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	fmt.Printf("Profile saved to: %s\n", outputPath)
	return nil
}

// Analyze log file
func analyzeLogs(inputPath, profileName string) error {
	fmt.Printf("Analyzing: %s\n", inputPath)

	// Setup logger
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	// Create Drain3 engine
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}

	metricsRegistry := metrics.NewRegistry()
	engine := dedup.NewEngine(cfg, metricsRegistry, logger)

	// Read log file
	file, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	duplicateCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		entry := &ingestion.LogEntry{
			Message:   line,
			Timestamp: time.Now(),
		}

		result, err := engine.Process(entry)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to process line")
			continue
		}

		if result.IsDuplicate {
			duplicateCount++
		}

		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	// Get statistics
	stats := engine.GetStats()

	// Print analysis
	fmt.Println("\n=== Log Analysis Results ===")
	fmt.Printf("Total lines:        %d\n", lineCount)
	fmt.Printf("Unique patterns:    %d\n", stats.PatternCount)
	fmt.Printf("Duplicate lines:    %d\n", duplicateCount)

	if lineCount > 0 {
		dedupeRatio := float64(duplicateCount) / float64(lineCount) * 100
		savings := float64(lineCount-duplicateCount) / float64(lineCount) * 100
		fmt.Printf("Deduplication:      %.2f%% duplicates\n", dedupeRatio)
		fmt.Printf("Storage savings:    %.2f%%\n", 100-savings)
	}

	return nil
}

// List available profiles
func listProfiles() error {
	logger := zerolog.Nop()
	profileCfg := config.ProfilesConfig{
		AutoDetect:     true,
		LocalPath:      "./profiles",
		DefaultProfile: "generic",
	}

	manager := profiles.NewManager(profileCfg, logger)
	if err := manager.LoadProfiles(); err != nil {
		return fmt.Errorf("failed to load profiles: %w", err)
	}

	profileList := manager.ListProfiles()

	fmt.Println("Available profiles:")
	for _, name := range profileList {
		fmt.Printf("  - %s\n", name)
	}

	return nil
}

// Validate a profile file
func validateProfile(profilePath string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to read profile: %w", err)
	}

	var profile map[string]interface{}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	// Basic validation
	requiredFields := []string{"apiVersion", "kind", "metadata", "spec"}
	for _, field := range requiredFields {
		if _, ok := profile[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Pretty print profile
	if verbose {
		output, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format profile: %w", err)
		}
		fmt.Println(string(output))
	}

	return nil
}

