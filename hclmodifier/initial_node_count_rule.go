package hclmodifier

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types" // Import for type definitions
)

// InitialNodeCountRuleDefinition is a placeholder for the InitialNodeCountRule because its logic,
// which involves iterating over `node_pool` sub-blocks and conditionally removing attributes,
// is handled by the direct implementation in ApplyInitialNodeCountRule.
// This definition is not meant to be used by the generic ApplyRules engine.
//
// What it does (effectively via ApplyInitialNodeCountRule): For each `node_pool` within a `google_container_cluster`,
// it removes the `initial_node_count` attribute if it exists. It doesn't matter if `node_count` is present or not.
//
// Why it's necessary for GKE imports: After importing a GKE cluster, `node_pool` blocks might contain
// `initial_node_count`. While this attribute is used for creation, for existing node pools (especially those
// managed by autoscaling or with a `node_count` attribute), `initial_node_count` can be problematic.
// It can cause diffs if the current node count (managed by `node_count` or autoscaler) doesn't match,
// or it might attempt to resize the node pool on apply if `node_count` is not set.
// Removing `initial_node_count` defers to `node_count` or autoscaling for managing the number of nodes,
// which is generally the desired state for imported and ongoing management.
var InitialNodeCountRuleDefinition = types.Rule{ // Use types.Rule
	Name:               "Initial Node Count Rule: Remove initial_node_count from node_pools (handled by ApplyInitialNodeCountRule)",
	TargetResourceType: "google_container_cluster",
	// Conditions and Actions are omitted as this rule is not processed by the generic engine.
}

// ApplyInitialNodeCountRule removes the `initial_node_count` attribute from all `node_pool` blocks
// within a `google_container_cluster` resource. This is to prevent conflicts or unintended resize
// operations after a cluster import, as `node_count` or autoscaling should manage the node count
// for existing node pools.
//
// This function is called directly and does not use the generic Rule engine due to its specific
// iteration and logic for `node_pool` sub-blocks.
func (m *Modifier) ApplyInitialNodeCountRule() (modifications int, err error) {
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
