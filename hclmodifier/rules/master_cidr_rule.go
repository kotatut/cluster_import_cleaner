package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// MasterCIDRRuleDefinition defines a rule for handling potential conflicts related to master IP configuration
// in `google_container_cluster` resources, specifically when a cluster is private.
//
// What it does: It checks if a `google_container_cluster` resource has both a `master_ipv4_cidr_block` defined
// and a `private_cluster_config` block that contains a `private_endpoint_subnetwork` attribute.
// If all these conditions are met, it removes the `private_endpoint_subnetwork` attribute from the
// `private_cluster_config` block.
//
// Why it's necessary for GKE imports: When importing a private GKE cluster, Terraform might generate
// configuration that includes both `master_ipv4_cidr_block` and `private_cluster_config.private_endpoint_subnetwork`.
// The `master_ipv4_cidr_block` is used to specify the IP range for the master, and for private clusters,
// the master's endpoint is within this range and typically doesn't need a separate subnetwork defined via
// `private_endpoint_subnetwork`. Including `private_endpoint_subnetwork` can be redundant or even lead to
// configuration errors if it's not set to the correct master IP or if it's not actually needed.
// This rule simplifies the configuration for private clusters by removing this potentially problematic attribute
// when `master_ipv4_cidr_block` is already specified.
var MasterCIDRRuleDefinition = types.Rule{
	Name:               "Master CIDR Rule: Remove private_endpoint_subnetwork if master_ipv4_cidr_block and private_cluster_config exist",
	TargetResourceType: "google_container_cluster",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"master_ipv4_cidr_block"},
		},
		{
			Type: types.BlockExists,
			Path: []string{"private_cluster_config"},
		},
		{
			Type: types.AttributeExists,
			Path: []string{"private_cluster_config", "private_endpoint_subnetwork"},
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"private_cluster_config", "private_endpoint_subnetwork"},
		},
	},
}
