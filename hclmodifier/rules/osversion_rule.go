package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

var OsVersionRuleDefinition = types.Rule{
	Name:               "It's not mandatory but nice to clean up osversion to avoid cluster update on apply",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists,
			Path: []string{"node_config"},
		},
		{
			Type: types.BlockExists,
			Path: []string{"node_config", "windows_node_config"},
		},
		{
			Type: types.AttributeDoesntExist,
			Path: []string{"node_config", "windows_node_config", "osversion"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveBlock,
			Path: []string{"node_config", "windows_node_config"},
		},
	},
}

var OsVersionNodePoolRuleDefinition = types.Rule{
	Name:                  "It's not mandatory but nice to clean up osversion of node pools as well",
	TargetResourceType:    "google_container_cluster",
	ExecutionType:         types.RuleExecutionForEachNestedBlock,
	NestedBlockTargetType: "node_pool",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists,
			Path: []string{"node_config"},
		},
		{
			Type: types.BlockExists,
			Path: []string{"node_config", "windows_node_config"},
		},
		{
			Type: types.AttributeDoesntExist,
			Path: []string{"node_config", "windows_node_config", "osversion"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveBlock,
			Path: []string{"node_config", "windows_node_config"},
		},
	},
}
