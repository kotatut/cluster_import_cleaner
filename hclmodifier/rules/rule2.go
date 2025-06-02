package rules

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/GoogleCloudPlatform/hcl-modifier/pkg/hclmodifier"
)

// Rule2Definition defines the logic for Rule 2.
var Rule2Definition = hclmodifier.Rule{
	Name:               "Rule 2: Remove services_ipv4_cidr_block if cluster_secondary_range_name exists in ip_allocation_policy",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.BlockExists, // Ensure ip_allocation_policy block exists first
			Path: []string{"ip_allocation_policy"},
		},
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"ip_allocation_policy", "services_ipv4_cidr_block"},
		},
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"ip_allocation_policy", "cluster_secondary_range_name"},
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type: hclmodifier.RemoveAttribute,
			Path: []string{"ip_allocation_policy", "services_ipv4_cidr_block"},
		},
	},
}

// ApplyRule2 implements the logic for Rule 2 using the generic ApplyRules engine.
// Original detailed comments moved to Rule2Definition.
// 3. For each such block:
//    a. Find the `ip_allocation_policy` nested block.
//    b. If `ip_allocation_policy` block exists:
//        i. Check for `services_ipv4_cidr_block` attribute.
//        ii. Check for `cluster_secondary_range_name` attribute.
//        iii. If both attributes are found, remove `services_ipv4_cidr_block`.
//        iv. Increment counter.
// 4. Log information.
// 5. Return total modifications and any error.
func (m *hclmodifier.Modifier) ApplyRule2() (modifications int, err error) {
	m.Logger.Info("Applying Rule 2 using the generic ApplyRules engine.")
	mods, errs := m.ApplyRules([]hclmodifier.Rule{Rule2Definition})
	if len(errs) > 0 {
		m.Logger.Error("Error(s) applying Rule2Definition.", zap.Errors("errors", errs))
		return mods, fmt.Errorf("errors applying Rule2Definition: %v", errs)
	}
	m.Logger.Info("Rule 2 applied successfully.", zap.Int("modifications", mods))
	return mods, nil
}
