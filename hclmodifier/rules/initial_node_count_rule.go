package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// InitialNodeCountRuleDefinition defines a rule for removing the `initial_node_count` attribute
// from `node_pool` blocks within `google_container_cluster` resources.
//
// What it does: It iterates over each `node_pool` block within a `google_container_cluster`
// resource and removes the `initial_node_count` attribute if it exists.
//
// Why it's necessary for GKE imports: When importing existing GKE clusters, `initial_node_count`
// is often present but is not recommended for use with node pools managed by Terraform after import,
// especially if autoscaling or `node_count` is also defined. Removing it helps avoid potential
// conflicts or unintended behavior during updates.
var InitialNodeCountRuleDefinition = types.Rule{
	Name:                  "InitialNodeCount Rule: Remove initial_node_count from node_pools",
	TargetResourceType:    "google_container_cluster",
	ExecutionType:         types.RuleExecutionForEachNestedBlock,
	NestedBlockTargetType: "node_pool",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"initial_node_count"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"initial_node_count"},
		},
	},
}
