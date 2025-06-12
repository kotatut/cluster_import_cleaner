package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// PodIPV4CIDRRuleDefinition defines a rule for handling conflicts within the `ip_allocation_policy` block
// related to pod IP range definition for `google_container_cluster` resources.
//
// What it does: It checks if a `google_container_cluster` resource's `ip_allocation_policy` block contains
// both the `cluster_ipv4_cidr_block` attribute (for pods) and the `cluster_secondary_range_name` attribute (for pods).
// If both are present, it removes the `cluster_ipv4_cidr_block` attribute.
//
// Why it's necessary for GKE imports: When importing a GKE cluster, Terraform might populate both
// `cluster_ipv4_cidr_block` and `cluster_secondary_range_name` for the pod IP range.
// Using `cluster_secondary_range_name` is often preferred when using VPC-native clusters with an existing
// secondary range for pods. Defining the CIDR block directly via `cluster_ipv4_cidr_block` can
// conflict with the named secondary range or be redundant. This rule standardizes on using the named
// secondary range for pods by removing the direct CIDR block definition in such cases.
var PodIPV4CIDRRuleDefinition = types.Rule{
	Name:               "Pod IPV4 CIDR Rule: Remove cluster_ipv4_cidr_block if cluster_secondary_range_name exists in ip_allocation_policy",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists, // Ensure ip_allocation_policy block exists first
			Path: []string{"ip_allocation_policy"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"ip_allocation_policy", "cluster_ipv4_cidr_block"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"ip_allocation_policy", "cluster_secondary_range_name"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"ip_allocation_policy", "cluster_ipv4_cidr_block"},
		},
	},
}
