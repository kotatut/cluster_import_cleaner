package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// BinaryAuthorizationRuleDefinition defines a rule for handling conflicts within the `binary_authorization`
// block of `google_container_cluster` resources.
//
// What it does: It checks if a `google_container_cluster` resource's `binary_authorization` block contains
// both the `enabled` attribute and the `evaluation_mode` attribute. If both are present, it removes
// the `enabled` attribute.
//
// Why it's necessary for GKE imports: When a GKE cluster with Binary Authorization is imported,
// Terraform configuration might include both `enabled` and `evaluation_mode` in the `binary_authorization` block.
// The `evaluation_mode` attribute (e.g., "PROJECT_SINGLETON_POLICY_ENFORCE" or "DISABLED") is generally
// sufficient to control the state of Binary Authorization. The `enabled` attribute can be redundant and
// potentially lead to conflicts or misunderstandings. This rule simplifies the configuration by removing
// `enabled` when `evaluation_mode` is present, relying on `evaluation_mode` as the source of truth.
var BinaryAuthorizationRuleDefinition = types.Rule{
	Name:               "Binary Authorization Rule: Remove 'enabled' attribute if 'evaluation_mode' also exists in binary_authorization block",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists,
			Path: []string{"binary_authorization"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"binary_authorization", "enabled"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"binary_authorization", "evaluation_mode"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"binary_authorization", "enabled"},
		},
	},
}
