package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// SetMinVersionRuleDefinition defines a rule for handling conflicts within the `node_version`
// and `min_master_version` root attributes of `google_container_cluster` resources.
//
// Why it's necessary for GKE imports: `min_master_version` is automatically selected if not set implicitly
// but it has to match `node_version` according to documentation.
var SetMinVersionRuleDefinition = types.Rule{
	Name:               "Set Min Master Version Rule: set it to node_version if it presents",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"node_version"},
		},
		{
			Type: types.AttributeDoesntExists,
			Path: []string{"min_master_version"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type:      types.SetAttributeValue,
			Path:      []string{"min_master_version"},
			PathToSet: []string{"node_version"},
		},
	},
}
