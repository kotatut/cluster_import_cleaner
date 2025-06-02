package rules

import (
	"fmt"

	"go.uber.org/zap" 

	"github.com/GoogleCloudPlatform/hcl-modifier/pkg/hclmodifier" // Import the hclmodifier package
)

// MasterCIDRRuleDefinition defines the logic for MasterCIDRRule.
var MasterCIDRRuleDefinition = hclmodifier.Rule{
	Name:               "MasterCIDRRule: Remove private_endpoint_subnetwork if master_ipv4_cidr_block and private_cluster_config exist",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"master_ipv4_cidr_block"},
		},
		{
			Type: hclmodifier.BlockExists,
			Path: []string{"private_cluster_config"},
		},
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"private_cluster_config", "private_endpoint_subnetwork"},
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type: hclmodifier.RemoveAttribute,
			Path: []string{"private_cluster_config", "private_endpoint_subnetwork"},
		},
	},
}

// ApplyMasterCIDRRule implements the logic for MasterCIDRRule using the generic ApplyRules engine.
// Original detailed comments moved to MasterCIDRRuleDefinition.
// 1. Initialize modificationCount to 0.
// 2. Log the start of the rule application.
// 3. Iterate through all 'resource' blocks of type 'google_container_cluster'.
// 4. For each cluster:
//    a. Check for 'master_ipv4_cidr_block' attribute.
//    b. Find 'private_cluster_config' nested block.
//    c. If found, check for 'private_endpoint_subnetwork' attribute within it.
//    d. If 'master_ipv4_cidr_block' and 'private_cluster_config.private_endpoint_subnetwork' exist,
//       remove 'private_endpoint_subnetwork'.
// 5. Log actions and increment modificationCount.
// 6. Log completion and return modificationCount.
func (m *hclmodifier.Modifier) ApplyMasterCIDRRule() (modifications int, err error) {
	m.Logger.Info("Applying MasterCIDRRule using the generic ApplyRules engine.")
	mods, errs := m.ApplyRules([]hclmodifier.Rule{MasterCIDRRuleDefinition})
	if len(errs) > 0 {
		m.Logger.Error("Error(s) applying MasterCIDRRuleDefinition.", zap.Errors("errors", errs))
		return mods, fmt.Errorf("errors applying MasterCIDRRuleDefinition: %v", errs)
	}
	m.Logger.Info("MasterCIDRRule applied successfully.", zap.Int("modifications", mods))
	return mods, nil
}
