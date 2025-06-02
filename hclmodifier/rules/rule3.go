package rules

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/GoogleCloudPlatform/hcl-modifier/pkg/hclmodifier"
)

// Rule3Definition defines the logic for Rule 3.
var Rule3Definition = hclmodifier.Rule{
	Name:               "Rule 3: Remove enabled if evaluation_mode exists in binary_authorization",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.BlockExists, // Ensure binary_authorization block exists first
			Path: []string{"binary_authorization"},
		},
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"binary_authorization", "enabled"},
		},
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"binary_authorization", "evaluation_mode"},
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type: hclmodifier.RemoveAttribute,
			Path: []string{"binary_authorization", "enabled"},
		},
	},
}

// ApplyRule3 implements the logic for Rule 3 using the generic ApplyRules engine.
// Original detailed comments moved to Rule3Definition.
// 3. For each such block:
//    a. Check for a nested block named `binary_authorization`.
//    b. If the `binary_authorization` block exists:
//        i. Check for an attribute named `enabled` within this nested block.
//        ii. Check for an attribute named `evaluation_mode` within this nested block.
//        iii. If both `enabled` and `evaluation_mode` attributes are found, remove the `enabled` attribute from the `binary_authorization` block.
//        iv. Increment a counter for each modification.
// 4. Log information about the process (e.g., "Starting ApplyRule3", "Found 'binary_authorization' block", "Removed 'enabled' attribute").
// 5. Return the total count of modifications and any error, similar to `ApplyRule1` and `ApplyRule2`.
func (m *hclmodifier.Modifier) ApplyRule3() (modifications int, err error) {
	m.Logger.Info("Applying Rule 3 using the generic ApplyRules engine.")
	mods, errs := m.ApplyRules([]hclmodifier.Rule{Rule3Definition})
	if len(errs) > 0 {
		m.Logger.Error("Error(s) applying Rule3Definition.", zap.Errors("errors", errs))
		return mods, fmt.Errorf("errors applying Rule3Definition: %v", errs)
	}
	m.Logger.Info("Rule 3 applied successfully.", zap.Int("modifications", mods))
	return mods, nil
}
