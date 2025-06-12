package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types" // Import for type definitions
)

// InitialNodeCountRuleDefinition defines a rule that removes the `initial_node_count` attribute
// from all `node_pool` sub-blocks within a `google_container_cluster` resource.
// This rule is processed by the generic ApplyRules engine using RuleExecutionForEachNestedBlock.
//
// What it does: For each `node_pool` block within a `google_container_cluster` resource,
// if the `initial_node_count` attribute exists within that `node_pool`, it will be removed.
//
// Why it's necessary for GKE imports: After importing a GKE cluster, `node_pool` blocks might contain
// `initial_node_count`. While this attribute is used for creation, for existing node pools (especially those
// managed by autoscaling or with a `node_count` attribute), `initial_node_count` can be problematic.
// Removing `initial_node_count` defers to `node_count` or autoscaling for managing the number of nodes.
var InitialNodeCountRuleDefinition = types.Rule{
	Name:                  "Initial Node Count Rule: Remove initial_node_count from node_pools",
	TargetResourceType:    "google_container_cluster",
	ExecutionType:         types.RuleExecutionForEachNestedBlock,
	NestedBlockTargetType: "node_pool",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"initial_node_count"}, // Path relative to the node_pool block
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"initial_node_count"}, // Path relative to the node_pool block
		},
	},
}
