package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

var DiskSizeRuleDefinition = types.Rule{
	Name:               "Clean up default 0 disk size",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists,
			Path: []string{"cluster_autoscaling"},
		},
		{
			Type: types.BlockExists,
			Path: []string{"cluster_autoscaling", "auto_provisioning_defaults"},
		},
		{
			Type:          types.AttributeValueEquals,
			Path:          []string{"cluster_autoscaling", "auto_provisioning_defaults", "disk_size"},
			ExpectedValue: "0",
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"cluster_autoscaling", "auto_provisioning_defaults", "disk_size"},
		},
	},
}
