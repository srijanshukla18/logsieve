package dedup

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
)

// Drain3Config represents the configuration for Drain3 algorithm
type Drain3Config struct {
	SimThreshold         float64  `mapstructure:"sim_th"`
	MaxNodeDepth         int      `mapstructure:"max_node_depth"`
	MaxChildren          int      `mapstructure:"max_children"`
	MaxClusters          int      `mapstructure:"max_clusters"`
	ParametrizeNumeric   bool     `mapstructure:"parametrize_numeric_tokens"`
	ExtraDelimiters      []string `mapstructure:"extra_delimiters"`
}

// DefaultDrain3Config returns default configuration
func DefaultDrain3Config() Drain3Config {
	return Drain3Config{
		SimThreshold:         0.4,
		MaxNodeDepth:         4,
		MaxChildren:          100,
		MaxClusters:          0, // 0 means unlimited
		ParametrizeNumeric:   true,
		ExtraDelimiters:      []string{"_", ":"},
	}
}

type Drain3 struct {
	config       Drain3Config
	logger       zerolog.Logger
	clusters     map[int]*LogCluster
	treeRoot     *Node
	clusterID    int
	mu           sync.RWMutex
	tokenRegex   *regexp.Regexp
	maskingRules []MaskingRule
}

// LogCluster represents a log template cluster
type LogCluster struct {
	ClusterID    int       `json:"cluster_id"`
	LogTemplate  []string  `json:"log_template_tokens"`
	Size         int       `json:"size"`
	Created      time.Time `json:"created"`
	Updated      time.Time `json:"updated"`
}

// Node represents a node in the prefix tree
type Node struct {
	Children map[string]*Node
	Clusters []*LogCluster
	Depth    int
}

// MaskingRule represents a regex-based masking rule
type MaskingRule struct {
	Pattern *regexp.Regexp
	Mask    string
}

// AddLogResult represents the result of adding a log message
type AddLogResult struct {
	ChangeType     string `json:"change_type"`
	ClusterID      int    `json:"cluster_id"`
	ClusterSize    int    `json:"cluster_size"`
	ClusterCount   int    `json:"cluster_count"`
	TemplateMined  string `json:"template_mined"`
}

// TemplateMatch represents a match result for inference mode
type TemplateMatch struct {
	ClusterID     int      `json:"cluster_id"`
	Template      []string `json:"template"`
	Similarity    float64  `json:"similarity"`
	ParameterList []string `json:"parameter_list"`
}

// ExtractedParameter represents an extracted parameter with its mask name
type ExtractedParameter struct {
	Value    string `json:"value"`
	MaskName string `json:"mask_name"`
}

func NewDrain3(config config.DedupConfig, logger zerolog.Logger) *Drain3 {
	drain3Config := DefaultDrain3Config()
	
	// Override with provided config values
	if config.SimilarityThreshold > 0 {
		drain3Config.SimThreshold = config.SimilarityThreshold
	}
	
	tokenRegex := regexp.MustCompile(`\S+`)
	
	// Initialize default masking rules (Go-compatible regex patterns)
	maskingRules := []MaskingRule{
		{
			Pattern: regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
			Mask:    "IP",
		},
		{
			Pattern: regexp.MustCompile(`\b[\-\+]?\d+\b`),
			Mask:    "NUM",
		},
		{
			Pattern: regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`),
			Mask:    "UUID",
		},
		{
			Pattern: regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`),
			Mask:    "HEX",
		},
	}
	
	return &Drain3{
		config:       drain3Config,
		logger:       logger.With().Str("component", "drain3").Logger(),
		clusters:     make(map[int]*LogCluster),
		treeRoot:     &Node{Children: make(map[string]*Node), Depth: 0},
		clusterID:    0,
		tokenRegex:   tokenRegex,
		maskingRules: maskingRules,
	}
}

// AddLogMessage processes a log message in training mode (official Drain3 API)
func (d *Drain3) AddLogMessage(logMessage string) *AddLogResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	tokens := d.tokenize(logMessage)
	if len(tokens) == 0 {
		return &AddLogResult{
			ChangeType:    "none",
			ClusterCount:  len(d.clusters),
		}
	}

	cluster := d.treeSearch(tokens)
	
    if cluster != nil {
        // Update existing cluster
        oldTemplate := strings.Join(cluster.LogTemplate, " ")
        cluster.Size++
        cluster.Updated = time.Now()
        
        // Update template if needed (generalize with wildcards)
        d.updateTemplate(cluster, tokens)
        newTemplate := strings.Join(cluster.LogTemplate, " ")
        
        changeType := "none"
        if oldTemplate != newTemplate {
            changeType = "cluster_template_changed"
        } else {
            changeType = "cluster_size_changed"
        }
        
        return &AddLogResult{
            ChangeType:     changeType,
            ClusterID:      cluster.ClusterID,
            ClusterSize:    cluster.Size,
            ClusterCount:   len(d.clusters),
            TemplateMined:  newTemplate,
        }
    }

	// Create new cluster
	newCluster := d.createCluster(tokens)
	d.addClusterToTree(newCluster, tokens)
	
    return &AddLogResult{
        ChangeType:     "cluster_created",
        ClusterID:      newCluster.ClusterID,
        ClusterSize:    newCluster.Size,
        ClusterCount:   len(d.clusters),
        TemplateMined:  strings.Join(newCluster.LogTemplate, " "),
    }
}

// Match performs inference mode matching (official Drain3 API)
func (d *Drain3) Match(logMessage string, fullSearchStrategy bool) *TemplateMatch {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tokens := d.tokenize(logMessage)
	if len(tokens) == 0 {
		return nil
	}

	cluster := d.treeSearch(tokens)
	if cluster == nil {
		return nil
	}

	similarity := d.calculateSimilarity(cluster.LogTemplate, tokens)
	if similarity < d.config.SimThreshold {
		return nil
	}

	parameters := d.extractParametersFromTokens(cluster.LogTemplate, tokens)
	
	return &TemplateMatch{
		ClusterID:     cluster.ClusterID,
		Template:      cluster.LogTemplate,
		Similarity:    similarity,
		ParameterList: parameters,
	}
}

// ExtractParameters extracts parameters from a log message based on a template (official Drain3 API)
func (d *Drain3) ExtractParameters(logMessage string, template string) []ExtractedParameter {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tokens := d.tokenize(logMessage)
	templateTokens := strings.Fields(template)
	
	if len(tokens) != len(templateTokens) {
		return nil
	}
	
	var parameters []ExtractedParameter
	for i, templateToken := range templateTokens {
		if strings.HasPrefix(templateToken, "<") && strings.HasSuffix(templateToken, ">") {
			maskName := strings.Trim(templateToken, "<>")
			if maskName == "*" {
				maskName = "*"
			}
			parameters = append(parameters, ExtractedParameter{
				Value:    tokens[i],
				MaskName: maskName,
			})
		}
	}
	
	return parameters
}

// Process maintains backward compatibility with existing LogSieve code
func (d *Drain3) Process(message string) (string, bool) {
	result := d.AddLogMessage(message)
	return strconv.Itoa(result.ClusterID), result.ChangeType == "cluster_created"
}

func (d *Drain3) tokenize(message string) []string {
	// Apply extra delimiters if configured
	processedMessage := message
	for _, delimiter := range d.config.ExtraDelimiters {
		processedMessage = strings.ReplaceAll(processedMessage, delimiter, " ")
	}
	
	tokens := d.tokenRegex.FindAllString(processedMessage, -1)
	
	// Apply masking rules
	for i, token := range tokens {
		tokens[i] = d.applyMasking(token)
	}
	
	return tokens
}

func (d *Drain3) applyMasking(token string) string {
	// Apply masking rules in order
	for _, rule := range d.maskingRules {
		if rule.Pattern.MatchString(token) {
			return fmt.Sprintf("<%s>", rule.Mask)
		}
	}
	
	// Parametrize numeric tokens if enabled
	if d.config.ParametrizeNumeric {
		if matched, _ := regexp.MatchString(`^\d+$`, token); matched {
			return "<NUM>"
		}
	}
	
	return token
}

// treeSearch searches for the best matching cluster using the prefix tree
func (d *Drain3) treeSearch(tokens []string) *LogCluster {
	if len(tokens) == 0 {
		return nil
	}

	// Start from root
	currentNode := d.treeRoot
	currentDepth := 0
	
	// First level: group by log length
	lengthKey := strconv.Itoa(len(tokens))
	if currentNode.Children[lengthKey] == nil {
		return nil
	}
	currentNode = currentNode.Children[lengthKey]
	currentDepth++
	
	// Traverse tree based on first few tokens (up to max_node_depth)
	for currentDepth < d.config.MaxNodeDepth && currentDepth-1 < len(tokens) {
		tokenIndex := currentDepth - 1
		token := tokens[tokenIndex]
		
		if currentNode.Children[token] != nil {
			currentNode = currentNode.Children[token]
		} else if currentNode.Children["<*>"] != nil {
			currentNode = currentNode.Children["<*>"]
		} else {
			return nil
		}
		currentDepth++
	}
	
	// Find best matching cluster in leaf clusters
	var bestCluster *LogCluster
	var bestSimilarity float64
	
	for _, cluster := range currentNode.Clusters {
		similarity := d.calculateSimilarity(cluster.LogTemplate, tokens)
		if similarity > bestSimilarity && similarity >= d.config.SimThreshold {
			bestSimilarity = similarity
			bestCluster = cluster
		}
	}
	
	return bestCluster
}

func (d *Drain3) calculateSimilarity(template []string, tokens []string) float64 {
	if len(template) != len(tokens) {
		return 0.0
	}
	
	matches := 0
	for i := 0; i < len(template); i++ {
		if template[i] == tokens[i] || template[i] == "<*>" {
			matches++
		}
	}
	
	return float64(matches) / float64(len(template))
}

func (d *Drain3) createCluster(tokens []string) *LogCluster {
	d.clusterID++
	
	templateTokens := make([]string, len(tokens))
	copy(templateTokens, tokens)
	
	return &LogCluster{
		ClusterID:   d.clusterID,
		LogTemplate: templateTokens,
		Size:        1,
		Created:     time.Now(),
		Updated:     time.Now(),
	}
}

func (d *Drain3) addClusterToTree(cluster *LogCluster, tokens []string) {
	d.clusters[cluster.ClusterID] = cluster
	
	// Navigate to the appropriate leaf node
	currentNode := d.treeRoot
	currentDepth := 0
	
	// First level: group by log length
	lengthKey := strconv.Itoa(len(tokens))
	if currentNode.Children[lengthKey] == nil {
		currentNode.Children[lengthKey] = &Node{
			Children: make(map[string]*Node),
			Clusters: []*LogCluster{},
			Depth:    1,
		}
	}
	currentNode = currentNode.Children[lengthKey]
	currentDepth++
	
	// Traverse/create tree path based on first few tokens
	for currentDepth < d.config.MaxNodeDepth && currentDepth-1 < len(tokens) {
		tokenIndex := currentDepth - 1
		token := tokens[tokenIndex]
		
		if currentNode.Children[token] == nil {
			currentNode.Children[token] = &Node{
				Children: make(map[string]*Node),
				Clusters: []*LogCluster{},
				Depth:    currentDepth + 1,
			}
		}
		currentNode = currentNode.Children[token]
		currentDepth++
	}
	
	// Add cluster to leaf node
	currentNode.Clusters = append(currentNode.Clusters, cluster)
	
	// Enforce max_children limit
	if d.config.MaxChildren > 0 && len(currentNode.Clusters) > d.config.MaxChildren {
		// Remove oldest cluster (simple FIFO eviction)
		currentNode.Clusters = currentNode.Clusters[1:]
	}
}

func (d *Drain3) updateTemplate(cluster *LogCluster, tokens []string) {
    if len(cluster.LogTemplate) != len(tokens) {
        return
    }
    
    // Update template by generalizing differences with wildcards
    changed := false
    for i := 0; i < len(cluster.LogTemplate); i++ {
        if cluster.LogTemplate[i] != tokens[i] && cluster.LogTemplate[i] != "<*>" {
            cluster.LogTemplate[i] = "<*>"
            changed = true
        }
    }

    // If the template changed to include wildcards, reindex this cluster under a wildcard path
    if changed {
        d.addWildcardPathForCluster(cluster)
    }
}

// addWildcardPathForCluster ensures the cluster is reachable via a path that respects
// wildcard positions in its template (up to max node depth). This does not remove
// the cluster from previous leaves to avoid expensive rewrites.
func (d *Drain3) addWildcardPathForCluster(cluster *LogCluster) {
    // Navigate to the appropriate leaf node using wildcard tokens where present
    currentNode := d.treeRoot
    currentDepth := 0
    template := cluster.LogTemplate

    // First level: group by log length
    lengthKey := strconv.Itoa(len(template))
    if currentNode.Children[lengthKey] == nil {
        currentNode.Children[lengthKey] = &Node{
            Children: make(map[string]*Node),
            Clusters: []*LogCluster{},
            Depth:    1,
        }
    }
    currentNode = currentNode.Children[lengthKey]
    currentDepth++

    for currentDepth < d.config.MaxNodeDepth && currentDepth-1 < len(template) {
        idx := currentDepth - 1
        key := template[idx]
        if key != "<*>" {
            // Keep specific path as well
            if currentNode.Children[key] == nil {
                currentNode.Children[key] = &Node{Children: make(map[string]*Node), Clusters: []*LogCluster{}, Depth: currentDepth + 1}
            }
            // Also create a wildcard branch for this depth to allow generalized matching
            if currentNode.Children["<*>"] == nil {
                currentNode.Children["<*>"] = &Node{Children: make(map[string]*Node), Clusters: []*LogCluster{}, Depth: currentDepth + 1}
            }
            // Prefer wildcard branch for generalized path
            currentNode = currentNode.Children["<*>"]
        } else {
            if currentNode.Children["<*>"] == nil {
                currentNode.Children["<*>"] = &Node{Children: make(map[string]*Node), Clusters: []*LogCluster{}, Depth: currentDepth + 1}
            }
            currentNode = currentNode.Children["<*>"]
        }
        currentDepth++
    }

    // Attach cluster to this leaf if not already present
    exists := false
    for _, c := range currentNode.Clusters {
        if c == cluster {
            exists = true
            break
        }
    }
    if !exists {
        currentNode.Clusters = append(currentNode.Clusters, cluster)
    }
}

func (d *Drain3) extractParametersFromTokens(template []string, tokens []string) []string {
	if len(template) != len(tokens) {
		return nil
	}
	
	var parameters []string
	for i, templateToken := range template {
		if templateToken == "<*>" || strings.HasPrefix(templateToken, "<") && strings.HasSuffix(templateToken, ">") {
			parameters = append(parameters, tokens[i])
		}
	}
	
	return parameters
}

// GetCluster returns a cluster by ID
func (d *Drain3) GetCluster(id int) *LogCluster {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	return d.clusters[id]
}

// GetClusters returns all clusters
func (d *Drain3) GetClusters() []*LogCluster {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	clusters := make([]*LogCluster, 0, len(d.clusters))
	for _, cluster := range d.clusters {
		clusters = append(clusters, cluster)
	}
	
	return clusters
}

// GetPatternCount returns the number of patterns (clusters)
func (d *Drain3) GetPatternCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	return len(d.clusters)
}

// GetTopClusters returns the top clusters by size
func (d *Drain3) GetTopClusters(limit int) []*LogCluster {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	clusters := make([]*LogCluster, 0, len(d.clusters))
	for _, cluster := range d.clusters {
		clusters = append(clusters, cluster)
	}
	
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Size > clusters[j].Size
	})
	
	if limit > 0 && len(clusters) > limit {
		clusters = clusters[:limit]
	}
	
	return clusters
}

// Reset clears all clusters and resets the tree
func (d *Drain3) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.clusters = make(map[int]*LogCluster)
	d.treeRoot = &Node{Children: make(map[string]*Node), Depth: 0}
	d.clusterID = 0
}

// GetTemplate returns a template by string ID (for backward compatibility)
// Note: This converts the string ID to int and returns a Template-like structure
func (d *Drain3) GetTemplate(id string) *Template {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	// Try to convert string ID to int
	clusterID := 0
	if _, err := fmt.Sscanf(id, "%d", &clusterID); err != nil {
		return nil
	}
	
	cluster := d.clusters[clusterID]
	if cluster == nil {
		return nil
	}
	
	// Convert LogCluster to Template for backward compatibility
	return &Template{
		ID:       id,
		Template: cluster.LogTemplate,
		Count:    cluster.Size,
		Created:  cluster.Created,
		Updated:  cluster.Updated,
	}
}

// GetTopTemplates returns top templates (for backward compatibility)
func (d *Drain3) GetTopTemplates(limit int) []*Template {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	clusters := d.GetTopClusters(limit)
	templates := make([]*Template, len(clusters))
	
	for i, cluster := range clusters {
		templates[i] = &Template{
			ID:       fmt.Sprintf("%d", cluster.ClusterID),
			Template: cluster.LogTemplate,
			Count:    cluster.Size,
			Created:  cluster.Created,
			Updated:  cluster.Updated,
		}
	}
	
	return templates
}

// Template represents a backward-compatible template structure
type Template struct {
	ID       string
	Template []string
	Count    int
	Created  time.Time
	Updated  time.Time
}

// PrintTree prints the prefix tree structure (for debugging)
func (d *Drain3) PrintTree(showClusterDetails bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	d.printNode(d.treeRoot, "", showClusterDetails)
}

func (d *Drain3) printNode(node *Node, prefix string, showDetails bool) {
	if len(node.Clusters) > 0 {
		fmt.Printf("%sLeaf: %d clusters\n", prefix, len(node.Clusters))
		if showDetails {
			for _, cluster := range node.Clusters {
				fmt.Printf("%s  Cluster %d (size=%d): %s\n", 
					prefix, cluster.ClusterID, cluster.Size, 
					strings.Join(cluster.LogTemplate, " "))
			}
		}
	}
	
	for key, child := range node.Children {
		fmt.Printf("%s%s/\n", prefix, key)
		d.printNode(child, prefix+"  ", showDetails)
	}
}

// Stats returns statistics about the Drain3 instance
func (d *Drain3) Stats() Drain3Stats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	totalMessages := 0
	for _, cluster := range d.clusters {
		totalMessages += cluster.Size
	}
	
	return Drain3Stats{
		ClusterCount:  len(d.clusters),
		TotalMessages: totalMessages,
		TreeDepth:     d.calculateTreeDepth(d.treeRoot, 0),
	}
}

func (d *Drain3) calculateTreeDepth(node *Node, currentDepth int) int {
	maxDepth := currentDepth
	
	for _, child := range node.Children {
		depth := d.calculateTreeDepth(child, currentDepth+1)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	
	return maxDepth
}

type Drain3Stats struct {
	ClusterCount  int `json:"cluster_count"`
	TotalMessages int `json:"total_messages"`
	TreeDepth     int `json:"tree_depth"`
}
