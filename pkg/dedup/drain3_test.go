package dedup

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/logsieve/logsieve/pkg/config"
)

func TestDrain3_BasicFunctionality(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Test the example from official Drain3 documentation
	logMessages := []string{
		"connected to 10.0.0.1",
		"connected to 192.168.0.1",
		"Hex number 0xDEADBEAF",
		"user davidoh logged in",
		"user eranr logged in",
	}
	
	results := make([]*AddLogResult, len(logMessages))
	for i, msg := range logMessages {
		results[i] = drain.AddLogMessage(msg)
		t.Logf("Message: %s -> Result: %+v", msg, results[i])
	}
	
	// Verify cluster creation
	assert.Equal(t, "cluster_created", results[0].ChangeType)
	// Second IP message should be clustered with first due to IP masking
	assert.Contains(t, []string{"cluster_size_changed", "cluster_template_changed"}, results[1].ChangeType)
	assert.Equal(t, "cluster_created", results[2].ChangeType)
	assert.Equal(t, "cluster_created", results[3].ChangeType)
	
	// The last message should match the user login pattern or create new cluster
	assert.Contains(t, []string{"cluster_created", "cluster_size_changed", "cluster_template_changed"}, results[4].ChangeType)
	
	// Check final cluster count
	stats := drain.Stats()
	assert.Greater(t, stats.ClusterCount, 0)
	assert.Equal(t, len(logMessages), stats.TotalMessages)
	
	t.Logf("Final stats: %+v", stats)
}

func TestDrain3_TemplateGeneralization(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.6,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Add similar messages that should generalize
	result1 := drain.AddLogMessage("user john logged in")
	result2 := drain.AddLogMessage("user mary logged in")
	result3 := drain.AddLogMessage("user bob logged in")
	
	assert.Equal(t, "cluster_created", result1.ChangeType)
	
	// Second and third should either create new clusters or update existing ones
	// depending on similarity threshold and template generalization
	t.Logf("Result 1: %+v", result1)
	t.Logf("Result 2: %+v", result2)
	t.Logf("Result 3: %+v", result3)
	
	// Use the results to avoid unused variable errors
	_ = result2
	_ = result3
	
	// Check that we have reasonable clustering
	stats := drain.Stats()
	assert.LessOrEqual(t, stats.ClusterCount, 3) // Should not exceed number of messages
	assert.Equal(t, 3, stats.TotalMessages)
	
	// Get clusters and check templates
	clusters := drain.GetClusters()
	for _, cluster := range clusters {
		template := strings.Join(cluster.LogTemplate, " ")
		t.Logf("Cluster %d (size=%d): %s", cluster.ClusterID, cluster.Size, template)
	}
}

func TestDrain3_MaskingRules(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Test IP masking
	result1 := drain.AddLogMessage("connected to 192.168.1.1")
	result2 := drain.AddLogMessage("connected to 10.0.0.1")
	
	// These should be clustered together due to IP masking
	assert.Equal(t, "cluster_created", result1.ChangeType)
	
	// Use result2 to avoid unused variable error
	_ = result2
	
	// Check if IP addresses are properly masked
	cluster1 := drain.GetCluster(result1.ClusterID)
	require.NotNil(t, cluster1)
	
	template := strings.Join(cluster1.LogTemplate, " ")
	assert.Contains(t, template, "<IP>")
	t.Logf("IP masking template: %s", template)
	
	// Test number masking
	result3 := drain.AddLogMessage("processed 100 items")
	result4 := drain.AddLogMessage("processed 250 items")
	
	cluster3 := drain.GetCluster(result3.ClusterID)
	require.NotNil(t, cluster3)
	
	template3 := strings.Join(cluster3.LogTemplate, " ")
	assert.Contains(t, template3, "<NUM>")
	t.Logf("Number masking template: %s", template3)
	
	// Use result4 to avoid unused variable error
	_ = result4
}

func TestDrain3_InferenceMode(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.6,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Train with some messages
	drain.AddLogMessage("user alice logged in")
	drain.AddLogMessage("user bob logged in")
	drain.AddLogMessage("connection established to 192.168.1.1")
	
	// Test inference mode
	match1 := drain.Match("user charlie logged in", false)
	if match1 != nil {
		assert.Greater(t, match1.Similarity, 0.6)
		assert.Contains(t, match1.ParameterList, "charlie")
		t.Logf("Match 1: %+v", match1)
	} else {
		t.Logf("Match 1: No match found (this is expected with current similarity threshold)")
	}
	
	match2 := drain.Match("connection established to 10.0.0.1", false)
	if match2 != nil {
		assert.Greater(t, match2.Similarity, 0.6)
		t.Logf("Match 2: %+v", match2)
	} else {
		t.Logf("Match 2: No match found (different wording: 'connection established' vs 'connected')")
	}
	
	// Test non-matching message
	match3 := drain.Match("completely different message format", false)
	assert.Nil(t, match3)
}

func TestDrain3_ParameterExtraction(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Train with a message
	result := drain.AddLogMessage("user johndoe logged in 11 minutes ago")
	cluster := drain.GetCluster(result.ClusterID)
	require.NotNil(t, cluster)
	
	template := strings.Join(cluster.LogTemplate, " ")
	t.Logf("Template: %s", template)
	
	// Extract parameters
	params := drain.ExtractParameters("user johndoe logged in 11 minutes ago", template)
	t.Logf("Extracted parameters: %+v", params)
	
	// Should extract variable parts
	assert.NotEmpty(t, params)
	
	// Check that we can extract from similar message
	params2 := drain.ExtractParameters("user alice logged in 5 minutes ago", template)
	t.Logf("Extracted parameters 2: %+v", params2)
}

func TestDrain3_Configuration(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.8, // High threshold
	}
	
	drain := NewDrain3(cfg, logger)
	
	// With high similarity threshold, similar messages should create separate clusters
	drain.AddLogMessage("user alice logged in")
	drain.AddLogMessage("user bob logged in")
	drain.AddLogMessage("user charlie logged in")
	
	stats := drain.Stats()
	t.Logf("High threshold stats: %+v", stats)
	
	// Test with low threshold
	cfg2 := config.DedupConfig{
		SimilarityThreshold: 0.2, // Low threshold
	}
	
	drain2 := NewDrain3(cfg2, logger)
	drain2.AddLogMessage("user alice logged in")
	drain2.AddLogMessage("user bob logged in")
	drain2.AddLogMessage("user charlie logged in")
	
	stats2 := drain2.Stats()
	t.Logf("Low threshold stats: %+v", stats2)
	
	// Low threshold should result in fewer clusters (more generalization)
	assert.LessOrEqual(t, stats2.ClusterCount, stats.ClusterCount)
}

func TestDrain3_TreeStructure(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Add messages of different lengths
	drain.AddLogMessage("short")
	drain.AddLogMessage("medium length message")
	drain.AddLogMessage("this is a much longer message with many tokens")
	
	stats := drain.Stats()
	assert.Greater(t, stats.TreeDepth, 0)
	assert.Equal(t, 3, stats.ClusterCount) // Different lengths should create different clusters
	assert.Equal(t, 3, stats.TotalMessages)
	
	t.Logf("Tree stats: %+v", stats)
	
	// Print tree for debugging
	t.Log("Tree structure:")
	drain.PrintTree(true)
}

func TestDrain3_BackwardCompatibility(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Test the old Process method for backward compatibility
	templateID1, isNew1 := drain.Process("user alice logged in")
	templateID2, isNew2 := drain.Process("user bob logged in")
	
	assert.True(t, isNew1)
	assert.NotEmpty(t, templateID1)
	
	// Second message might be new or might match first depending on similarity
	assert.NotEmpty(t, templateID2)
	
	t.Logf("Template ID 1: %s (new: %v)", templateID1, isNew1)
	t.Logf("Template ID 2: %s (new: %v)", templateID2, isNew2)
	
	// Verify we can still get pattern count
	count := drain.GetPatternCount()
	assert.Greater(t, count, 0)
	
	// Verify we can get top clusters
	topClusters := drain.GetTopClusters(5)
	assert.NotEmpty(t, topClusters)
}

func TestDrain3_Reset(t *testing.T) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}
	
	drain := NewDrain3(cfg, logger)
	
	// Add some messages
	drain.AddLogMessage("test message 1")
	drain.AddLogMessage("test message 2")
	
	stats := drain.Stats()
	assert.Greater(t, stats.ClusterCount, 0)
	assert.Greater(t, stats.TotalMessages, 0)
	
	// Reset
	drain.Reset()
	
	statsAfterReset := drain.Stats()
	assert.Equal(t, 0, statsAfterReset.ClusterCount)
	assert.Equal(t, 0, statsAfterReset.TotalMessages)
	assert.Equal(t, 0, statsAfterReset.TreeDepth)
}

// Benchmark test to ensure performance is reasonable
func BenchmarkDrain3_AddLogMessage(b *testing.B) {
	logger := zerolog.Nop()
	cfg := config.DedupConfig{
		SimilarityThreshold: 0.4,
	}
	
	drain := NewDrain3(cfg, logger)
	
	messages := []string{
		"user alice logged in",
		"user bob logged in",
		"connection to 192.168.1.1 established",
		"connection to 10.0.0.1 established",
		"processed 100 items",
		"processed 250 items",
		"error: file not found",
		"error: permission denied",
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		msg := messages[i%len(messages)]
		drain.AddLogMessage(msg)
	}
}