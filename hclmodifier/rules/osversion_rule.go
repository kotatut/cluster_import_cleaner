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
			Type: types.NullValue,
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
