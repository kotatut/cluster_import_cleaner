package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// RuleHandleAutopilotFalse defines a rule to clean up 'enable_autopilot'
// when it is explicitly set to false.
//
// What it does: It checks if a 'google_container_cluster' resource has the
// 'enable_autopilot' attribute set to 'false'. If so, it removes the attribute.
//
// Why it's necessary for GKE imports: If 'enable_autopilot' is 'false', it's often
// redundant as it's the default behavior for standard clusters if the attribute is
// omitted entirely. Removing it simplifies the configuration.
var RuleHandleAutopilotFalse = types.Rule{
	Name:               "Autopilot Cleanup: Remove 'enable_autopilot' if explicitly set to false",
	TargetResourceType: "google_container_cluster",
	ExecutionType:      types.RuleExecutionStandard, // Or can be omitted for default
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"enable_autopilot"},
		},
		{
			Type:          types.AttributeValueEquals,
			Path:          []string{"enable_autopilot"},
			ExpectedValue: "false",
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"enable_autopilot"},
		},
	},
}
