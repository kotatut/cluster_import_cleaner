package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// SetMinVersionRuleDefinition defines a rule for handling conflicts within the `node_version`
// and `min_master_version` root attributes of `google_container_cluster` resources.
//
// Why it's necessary for GKE imports: `min_master_version` is automatically selected if not set implicitly
// but it has to match `node_version` according to documentation.
var SetMinVersionRule_WhenAbsentDefinition = types.Rule{
	Name:               "Set Min Master Version Rule: set it to node_version if min_master_version is absent",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"node_version"},
		},
		{
			Type: types.AttributeDoesntExist, // Ensure this type matches the one defined in types/types.go
			Path: []string{"min_master_version"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type:           types.SetAttributeValueFromAttribute,
			Path:           []string{"min_master_version"},
			ValueToSetPath: []string{"node_version"},
		},
	},
}

var SetMinVersionRule_WhenNullDefinition = types.Rule{
	Name:               "Set Min Master Version Rule: set it to node_version if min_master_version is null",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"node_version"},
		},
		{
			Type: types.AttributeExists, // Ensure min_master_version attribute exists to check its value
			Path: []string{"min_master_version"},
		},
		{
			Type: types.NullValue, // Check if the existing min_master_version is null
			Path: []string{"min_master_version"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type:           types.SetAttributeValueFromAttribute,
			Path:           []string{"min_master_version"},
			ValueToSetPath: []string{"node_version"},
		},
	},
}
