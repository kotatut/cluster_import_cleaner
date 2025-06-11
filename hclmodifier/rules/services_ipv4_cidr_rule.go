package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// ServicesIPV4CIDRRuleDefinition defines a rule for handling conflicts within the `ip_allocation_policy` block
// related to services IP range definition for `google_container_cluster` resources.
//
// What it does: It checks if a `google_container_cluster` resource's `ip_allocation_policy` block contains
// both the `services_ipv4_cidr_block` attribute and the `cluster_secondary_range_name` attribute for services.
// If both are present, it removes the `services_ipv4_cidr_block` attribute.
//
// Why it's necessary for GKE imports: When importing a GKE cluster, especially one using secondary ranges,
// Terraform might populate both `services_ipv4_cidr_block` and `cluster_secondary_range_name` (for services).
// Using `cluster_secondary_range_name` is often preferred when using VPC-native clusters with secondary ranges
// managed by GKE or defined elsewhere. Defining the CIDR block directly via `services_ipv4_cidr_block` can
// conflict with the named secondary range or be redundant. This rule standardizes on using the named
// secondary range by removing the direct CIDR block definition in such cases.
var ServicesIPV4CIDRRuleDefinition = types.Rule{
	Name:               "Services IPV4 CIDR Rule: Remove services_ipv4_cidr_block if cluster_secondary_range_name (for services) exists in ip_allocation_policy",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.BlockExists, // Ensure ip_allocation_policy block exists first
			Path: []string{"ip_allocation_policy"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"ip_allocation_policy", "services_ipv4_cidr_block"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"ip_allocation_policy", "cluster_secondary_range_name"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"ip_allocation_policy", "services_ipv4_cidr_block"},
		},
	},
}
