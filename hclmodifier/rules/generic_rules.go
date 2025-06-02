package rules

import (
	"github.com/GoogleCloudPlatform/hcl-modifier/pkg/hclmodifier"
)

// RuleRemoveLoggingService defines a rule to remove logging_service if cluster_telemetry.type is ENABLED.
var RuleRemoveLoggingService = hclmodifier.Rule{
	Name:               "Remove logging_service if cluster_telemetry.type is ENABLED",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"logging_service"},
		},
		{
			Type: hclmodifier.BlockExists,
			Path: []string{"cluster_telemetry"},
		},
		{
			Type:          hclmodifier.AttributeValueEquals,
			Path:          []string{"cluster_telemetry", "type"},
			ExpectedValue: "ENABLED",
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type: hclmodifier.RemoveAttribute,
			Path: []string{"logging_service"},
		},
	},
}

// RuleRemoveMonitoringService defines a rule to remove monitoring_service if monitoring_config exists.
var RuleRemoveMonitoringService = hclmodifier.Rule{
	Name:               "Remove monitoring_service if monitoring_config exists",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"monitoring_service"},
		},
		{
			Type: hclmodifier.BlockExists,
			Path: []string{"monitoring_config"},
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type: hclmodifier.RemoveAttribute,
			Path: []string{"monitoring_service"},
		},
	},
}

// RuleRemoveNodeVersion defines a rule to remove node_version if min_master_version is also present.
var RuleRemoveNodeVersion = hclmodifier.Rule{
	Name:               "Remove node_version if min_master_version exists",
	TargetResourceType: "google_container_cluster",
	Conditions: []hclmodifier.RuleCondition{
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"node_version"},
		},
		{
			Type: hclmodifier.AttributeExists,
			Path: []string{"min_master_version"},
		},
	},
	Actions: []hclmodifier.RuleAction{
		{
			Type: hclmodifier.RemoveAttribute,
			Path: []string{"node_version"},
		},
	},
}
