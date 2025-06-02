package rules

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/GoogleCloudPlatform/hcl-modifier/pkg/hclmodifier" // Import the hclmodifier package
)

// Rule1Definition defines the logic for Rule 1 as a struct.
// Rule 1: If both `cluster_ipv4_cidr` (main block) and `ip_allocation_policy.cluster_ipv4_cidr_block` (nested) are found,
// remove `cluster_ipv4_cidr` from the main block for "google_container_cluster" resources.
var Rule1Definition = hclmodifier.Rule{
	Name:               "Rule 1: Remove cluster_ipv4_cidr if ip_allocation_policy.cluster_ipv4_cidr_block exists",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"cluster_ipv4_cidr"},
		},
		{
			Type: hclmodifier.BlockExists,
			Path: []string{"ip_allocation_policy"},
		},
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"ip_allocation_policy", "cluster_ipv4_cidr_block"},
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type: hclmodifier.RemoveAttribute,
			Path: []string{"cluster_ipv4_cidr"},
		},
	},
}

// ApplyRule1 implements the logic for Rule 1 using the generic ApplyRules engine.
// Original detailed comments moved to Rule1Definition.
// 3. For each such block:
//    a. Check for `cluster_ipv4_cidr` attribute.
//    b. Check for `ip_allocation_policy` nested block.
//    c. If `ip_allocation_policy` exists, check for `cluster_ipv4_cidr_block` attribute within it.
//    d. If both `cluster_ipv4_cidr` (main block) and `ip_allocation_policy.cluster_ipv4_cidr_block` (nested) are found,
//       remove `cluster_ipv4_cidr` from the main block.
//    e. Increment a counter for each modification.
// 4. Log information about the process.
// 5. Return the total count of modifications and any error.
func (m *hclmodifier.Modifier) ApplyRule1() (modifications int, err error) {
	m.Logger.Info("Applying Rule 1 using the generic ApplyRules engine.")

	mods, errs := m.ApplyRules([]hclmodifier.Rule{Rule1Definition}) // Pass the rule definition
	if len(errs) > 0 {
		m.Logger.Error("Error(s) applying Rule1Definition.", zap.Errors("errors", errs))
		return mods, fmt.Errorf("errors applying Rule1Definition: %v", errs)
	}
	m.Logger.Info("Rule 1 applied successfully.", zap.Int("modifications", mods))
	return mods, nil
}
