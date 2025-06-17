package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// RemoveTopLevelInitialNodeCountRuleDefinition defines a rule to remove the top-level
// initial_node_count attribute from a google_container_cluster resource
// if node_pool blocks are also defined.
// NOTE: The condition `AnyNestedBlockOfTypeExists` is conceptual.
// If not supported by the rule engine's condition evaluation, this rule
// might behave as if only the AttributeExists condition for initial_node_count is present.
var RemoveTopLevelInitialNodeCountRuleDefinition = types.Rule{
	Name:               "RemoveTopLevelInitialNodeCountWhenNodePoolsExist",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"initial_node_count"},
		},
		{
			Type:            types.AnyNestedBlockOfTypeExists, // This is a conceptual/hypothetical condition type
			NestedBlockType: "node_pool",                    // Parameter for the conceptual condition
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"initial_node_count"},
		},
	},
}
