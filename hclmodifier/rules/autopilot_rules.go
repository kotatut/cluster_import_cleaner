package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// RuleHandleAutopilotFalse defines a rule to clean up 'enable_autopilot'
// when it is explicitly set to false.
//
// What it does: It checks if a 'google_container_cluster' resource has the
// 'enable_autopilot' attribute set to 'false'. If so, it removes the attribute.
//
// Why it's necessary for GKE imports: If 'enable_autopilot' is 'false', it's often
// redundant as it's the default behavior for standard clusters if the attribute is
// omitted entirely. Removing it simplifies the configuration.
var RuleHandleAutopilotFalse = types.Rule{
	Name:               "Autopilot Cleanup: Remove 'enable_autopilot' if explicitly set to false",
	TargetResourceType: "google_container_cluster",
	ExecutionType:      types.RuleExecutionStandard, // Or can be omitted for default
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"enable_autopilot"},
		},
		{
			Type:          types.AttributeValueEquals,
			Path:          []string{"enable_autopilot"},
			ExpectedValue: "false",
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"enable_autopilot"},
		},
	},
}

var AutopilotRules = []types.Rule{
	{
		Name:               "Autopilot Cleanup: Handle enable_autopilot = true",
		TargetResourceType: "google_container_cluster",
		Conditions: []types.RuleCondition{
			{
				Type:          types.AttributeValueEquals,
				Path:          []string{"enable_autopilot"},
				ExpectedValue: "true", // This will be parsed to cty.BoolVal(true)
			},
		},
		Actions: []types.RuleAction{
			// Remove top-level attributes
			{Type: types.RemoveAttribute, Path: []string{"cluster_ipv4_cidr"}},
			{Type: types.RemoveAttribute, Path: []string{"enable_shielded_nodes"}},
			{Type: types.RemoveAttribute, Path: []string{"remove_default_node_pool"}},
			{Type: types.RemoveAttribute, Path: []string{"default_max_pods_per_node"}},
			{Type: types.RemoveAttribute, Path: []string{"enable_intranode_visibility"}},

			// Remove top-level nested blocks
			{Type: types.RemoveBlock, Path: []string{"network_policy"}},
			// Remove all "node_pool" blocks
			{Type: types.RemoveAllBlocksOfType, BlockTypeToRemove: "node_pool"},

			// Remove sub-blocks from addons_config
			{Type: types.RemoveBlock, Path: []string{"addons_config", "network_policy_config"}},
			{Type: types.RemoveBlock, Path: []string{"addons_config", "dns_cache_config"}},
			{Type: types.RemoveBlock, Path: []string{"addons_config", "stateful_ha_config"}},

			// Remove attributes and sub-blocks from cluster_autoscaling
			{Type: types.RemoveAttribute, Path: []string{"cluster_autoscaling", "enabled"}},
			{Type: types.RemoveAllNestedBlocksMatchingPath, Path: []string{"cluster_autoscaling", "resource_limits"}},

			// Remove attributes from binary_authorization
			{Type: types.RemoveAttribute, Path: []string{"binary_authorization", "enabled"}},
		},
	},
	{
		Name:               "Autopilot Cleanup: Handle enable_autopilot is not boolean",
		TargetResourceType: "google_container_cluster",
		Conditions: []types.RuleCondition{
			{
				Type:                     types.AttributeTypeIsNot,
				Path:                     []string{"enable_autopilot"},
				ExpectedTypeFriendlyName: "bool",
			},
		},
		Actions: []types.RuleAction{
			{Type: types.RemoveAttribute, Path: []string{"enable_autopilot"}},
		},
	},
}

// RuleHandleAutopilotFalse is a separate rule that should already exist or be defined elsewhere.
// It handles the case where `enable_autopilot = false`.
// For the purpose of this refactoring, we assume it's correctly handled by another rule.
// Example (if it were to be defined here):
/*
var RuleHandleAutopilotFalse = types.Rule{
	Name:               "Autopilot Cleanup: Handle enable_autopilot = false",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type:          types.AttributeValueEquals,
			Path:          []string{"enable_autopilot"},
			ExpectedValue: "false",
		},
	},
	Actions: []types.RuleAction{
		{Type: types.RemoveAttribute, Path: []string{"enable_autopilot"}},
	},
}
*/
