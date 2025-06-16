package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// RuleTerraformLabel defines a rule to clean up label is set by terraform plan -generate-config
//
// Why it's necessary for GKE imports: Removes need of small update on apply for existing cluster.
var RuleTerraformLabel = types.Rule{
	Name:               "Remove resourceLabels added by terraform import",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists,
			Path: []string{"terraform_labels"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"terraform_labels", "goog-terraform-provisioned"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"terraform_labels", "goog-terraform-provisioned"},
		},
	},
}
