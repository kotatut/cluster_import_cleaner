package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types" // Import the types package
)

// ClusterIPV4CIDRRuleDefinition defines a rule for handling conflicts between the top-level `cluster_ipv4_cidr`
// attribute and the `cluster_ipv4_cidr_block` attribute within the `ip_allocation_policy` block for
// `google_container_cluster` resources.
//
// What it does: It checks if a `google_container_cluster` resource has both the `cluster_ipv4_cidr` attribute
// defined at the top level and the `cluster_ipv4_cidr_block` attribute inside an `ip_allocation_policy` block.
// If both are present, it removes the top-level `cluster_ipv4_cidr` attribute.
//
// Why it's necessary for GKE imports: When a GKE cluster is imported, Terraform configuration might end up
// with both `cluster_ipv4_cidr` and `ip_allocation_policy.cluster_ipv4_cidr_block` defined.
// While GCP might allow this, it can lead to confusion and potential conflicts, as `ip_allocation_policy`
// is the more modern and flexible way to define IP allocation. This rule cleans up the configuration
// by removing the redundant top-level attribute, favoring the one within `ip_allocation_policy`.
var ClusterIPV4CIDRRuleDefinition = types.Rule{
	Name:               "Cluster IPV4 CIDR Rule: Remove top-level cluster_ipv4_cidr if ip_allocation_policy.cluster_ipv4_cidr_block exists",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"cluster_ipv4_cidr"},
		},
		{
			Type: types.BlockExists,
			Path: []string{"ip_allocation_policy"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"ip_allocation_policy", "cluster_ipv4_cidr_block"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"cluster_ipv4_cidr"},
		},
	},
}
