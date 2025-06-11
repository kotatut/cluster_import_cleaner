package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier"
)

// SetMinVersionRuleDefinition defines a rule for handling conflicts within the `node_version`
// and `min_master_version` root attributes of `google_container_cluster` resources.
//
// What it does: It checks if a `google_container_cluster` resource's `node_version` is set
// and set `min_master_version` to the same value to avoid the confict.

// Why it's necessary for GKE imports: `min_master_version` is automatically selected if not set implicitly
// but it has to match `node_version` according to documentation.
var SetMinVersionRuleDefinition = hclmodifier.Rule{
	Name:               "Set Min Master Version Rule: set it to node_version if it presents",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"node_version"},
		},
		{
			Type: hclmodifier.AttributeDoesntExists,
			Path: []string{"min_master_version"},
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type:      hclmodifier.SetAttributeValue,
			Path:      []string{"min_master_version"},
			PathToSet: []string{"node_version"},
		},
	},
}
