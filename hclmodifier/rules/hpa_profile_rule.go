package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types" // Import the types package
)

var HpaProfileRuleDefinition = types.Rule{
	Name:               "Cluster pod's autoscaling HPA profile: Remove if set to HPA_PROFILE_UNSPECIFIED by import",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists,
			Path: []string{"pod_autoscaling"},
		},
		{
			Type:          types.AttributeValueEquals,
			Path:          []string{"pod_autoscaling", "hpa_profile"},
			ExpectedValue: "HPA_PROFILE_UNSPECIFIED",
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveBlock,
			Path: []string{"pod_autoscaling"},
		},
	},
}
