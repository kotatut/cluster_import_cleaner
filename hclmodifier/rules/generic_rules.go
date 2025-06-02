package rules

import (
	"github.com/kotatut/cluster_import_cleaner/hclmodifier"
)

// RuleRemoveLoggingService defines a rule for managing the `logging_service` attribute in `google_container_cluster` resources,
// particularly when Cloud Operations for GKE (with `cluster_telemetry.type = "ENABLED"`) is used.
//
// What it does: It checks if a `google_container_cluster` resource has the `logging_service` attribute defined
// and also has a `cluster_telemetry` block with `type` set to "ENABLED". If all conditions are met,
// it removes the `logging_service` attribute.
//
// Why it's necessary for GKE imports: When a GKE cluster is imported, it might have an explicit `logging_service`
// (e.g., "logging.googleapis.com/kubernetes") and also be configured for Cloud Operations for GKE through
// `cluster_telemetry.type = "ENABLED"`. When `cluster_telemetry.type = "ENABLED"` is set, GKE manages logging
// and monitoring services, making the explicit `logging_service` attribute redundant and potentially conflicting.
// This rule removes the `logging_service` to align with the managed Cloud Operations configuration.
var RuleRemoveLoggingService = hclmodifier.Rule{
	Name:               "Logging Service Rule: Remove logging_service if cluster_telemetry.type is ENABLED",
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

// RuleRemoveMonitoringService defines a rule for managing the `monitoring_service` attribute in `google_container_cluster`
// resources, especially when the `monitoring_config` block is present.
//
// What it does: It checks if a `google_container_cluster` resource has both the `monitoring_service` attribute
// and a `monitoring_config` block. If both are present, it removes the `monitoring_service` attribute.
//
// Why it's necessary for GKE imports: Imported GKE cluster configurations might include both an older
// `monitoring_service` attribute (e.g., "monitoring.googleapis.com/kubernetes") and the newer `monitoring_config`
// block (which allows more granular control, like managed Prometheus). The `monitoring_config` block is the
// preferred way to configure monitoring. Having both can be confusing or lead to issues. This rule
// cleans up the configuration by removing the legacy `monitoring_service` when `monitoring_config` is used.
var RuleRemoveMonitoringService = hclmodifier.Rule{
	Name:               "Monitoring Service Rule: Remove monitoring_service if monitoring_config block exists",
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

// RuleRemoveNodeVersion defines a rule for managing GKE cluster and node pool version attributes.
//
// What it does: It checks if a `google_container_cluster` resource has both `node_version` and
// `min_master_version` attributes defined at the cluster level. If both are present, it removes
// the `node_version` attribute.
//
// Why it's necessary for GKE imports: When importing a GKE cluster, Terraform might populate both
// `min_master_version` (for the control plane) and `node_version` (for the default node pool, or as a
// cluster-wide default for node pools). However, GKE generally manages node pool versions relative
// to the master version, or allows them to be managed independently per node pool. Defining a cluster-level
// `node_version` can sometimes conflict with node pool specific versions or GKE's auto-upgrade mechanisms
// for node pools, especially when `min_master_version` is also set. This rule aims to simplify version
// management by removing `node_version` when `min_master_version` is present, encouraging version
// definition at the node pool level if specific versions are needed, or relying on GKE defaults.
var RuleRemoveNodeVersion = hclmodifier.Rule{
	Name:               "Node Version Rule: Remove cluster-level node_version if min_master_version also exists",
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
