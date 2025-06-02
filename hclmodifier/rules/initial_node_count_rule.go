package rules

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/GoogleCloudPlatform/hcl-modifier/pkg/hclmodifier"
)

// InitialNodeCountRuleDefinition is a placeholder for the InitialNodeCountRule.
// The current ApplyRules engine may not fully support its complex logic (iteration over node_pools).
// Thus, ApplyInitialNodeCountRule will continue to use its direct implementation.
var InitialNodeCountRuleDefinition = hclmodifier.Rule{
	Name:               "InitialNodeCountRule: Placeholder - Complex logic handled by ApplyInitialNodeCountRule",
	TargetResourceType: "google_container_cluster",
}

// ApplyInitialNodeCountRule implements the logic for managing 'initial_node_count'
// and 'node_count' attributes within 'node_pool' blocks of 'google_container_cluster' resources.
// 1. Initialize modificationCount to 0.
// 2. Log the start of the rule application.
// 3. Iterate through all 'resource' blocks of type 'google_container_cluster'.
// 4. For each cluster, iterate through its 'node_pool' nested blocks.
// 5. If both 'initial_node_count' and 'node_count' exist, remove 'initial_node_count'.
// 6. If only 'initial_node_count' exists, remove it.
// 7. Log actions and increment modificationCount.
// 8. Log completion and return modificationCount.
func (m *hclmodifier.Modifier) ApplyInitialNodeCountRule() (modifications int, err error) {
	modificationCount := 0
	var firstError error
	m.Logger.Info("Starting ApplyInitialNodeCountRule (using path-based helpers).")

	if m.File() == nil || m.File().Body() == nil { 
		m.Logger.Error("ApplyInitialNodeCountRule: Modifier's file or file body is nil.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	for _, block := range m.File().Body().Blocks() {
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == "google_container_cluster" {
			resourceName := block.Labels()[1]
			clusterLogger := m.Logger.With(zap.String("resourceName", resourceName), zap.String("rule", "ApplyInitialNodeCountRule"))
			clusterLogger.Debug("Checking 'google_container_cluster' resource.")

			for _, nodePoolBlock := range block.Body().Blocks() {
				if nodePoolBlock.Type() == "node_pool" {
					nodePoolLogger := clusterLogger.With(zap.String("nodePoolType", nodePoolBlock.Type())) 
					nodePoolLogger.Debug("Checking 'node_pool' block.")

					// Check if 'initial_node_count' exists in this node_pool block.
					_, _, errAttr := m.GetAttributeValueByPath(nodePoolBlock.Body(), []string{"initial_node_count"})

					if errAttr == nil { // Attribute exists
						nodePoolLogger.Info("Found 'initial_node_count' in node_pool. Removing it.")
						errRemove := m.RemoveAttributeByPath(nodePoolBlock.Body(), []string{"initial_node_count"})
						if errRemove != nil {
							nodePoolLogger.Error("Error removing 'initial_node_count' from node_pool.", zap.Error(errRemove))
							if firstError == nil {
								firstError = fmt.Errorf("failed to remove 'initial_node_count' from node_pool in resource '%s': %w", resourceName, errRemove)
							}
						} else {
							modificationCount++
							nodePoolLogger.Info("Successfully removed 'initial_node_count' from node_pool.")
						}
					} else {
						// GetAttributeValueByPath already logs debug messages for attribute not found.
						nodePoolLogger.Debug("Attribute 'initial_node_count' not found in this node_pool.", zap.Error(errAttr))
					}
				}
			}
		}
	}

	m.Logger.Info("ApplyInitialNodeCountRule finished.", zap.Int("modifications", modificationCount))
	if firstError != nil {
		m.Logger.Error("ApplyInitialNodeCountRule encountered errors during processing.", zap.Error(firstError))
	}
	return modificationCount, firstError
}
