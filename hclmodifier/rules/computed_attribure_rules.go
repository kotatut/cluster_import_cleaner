package rules

import (
	"fmt"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// TopLevelComputedAttributesRule defines a rule to clean computed attributes
// as 'label_fingerprint' or 'master_auth.cluster_ca_certificate'
//
// Why it's necessary for GKE imports: such attributes should not be used during apply since they are provided by GCP.
var TopLevelComputedAttributesRules = []types.Rule{
	createRemoveAttributeRule("google_container_cluster", []string{"endpoint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"effective_labels"}),
	createRemoveAttributeRule("google_container_cluster", []string{"id"}),
	createRemoveAttributeRule("google_container_cluster", []string{"label_fingerprint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"self_link"}),
}

var OtherComputedAttributesRules = []types.Rule{
	createRemoveAttributeRule("google_container_cluster", []string{"label_fingerprint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"self_link"}),
	createRemoveAttributeRule("google_container_cluster", []string{"master_auth", "cluster_ca_certificate"}),
	createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "instance_group_urls"}),
	createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "managed_instance_group_urls"}),
	createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "autoscaling", "total_max_node_count"}),
	createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "autoscaling", "total_min_node_count"}),
	createRemoveAttributeRule("google_container_cluster", []string{"private_cluster_config", "private_endpoint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"private_cluster_config", "public_endpoint"}),
}

func createRemoveAttributeRule(resourceType string, path []string) types.Rule {
	return types.Rule{
		Name:               fmt.Sprintf("Remove attribute '%s' from '%s'", path, resourceType),
		TargetResourceType: resourceType,
		Conditions: []types.RuleCondition{
			{
				Type: types.AttributeExists,
				Path: path,
			},
		},
		Actions: []types.RuleAction{
			{
				Type: types.RemoveAttribute,
				Path: path,
			},
		},
	}
}
