package types

import (
	"github.com/zclconf/go-cty/cty"
)

// ConditionType is an enumeration defining the types of conditions that can be checked by a Rule.
type ConditionType string

const (
	AttributeExists       ConditionType = "AttributeExists"       // AttributeExists checks if a specific attribute exists at the given path.
	AttributeDoesntExists ConditionType = "AttributeDoesntExists" // AttributeDoesntExists checks if a specific attribute is not present at the given path.
	BlockExists           ConditionType = "BlockExists"           // BlockExists checks if a specific block exists at the given path.
	AttributeValueEquals  ConditionType = "AttributeValueEquals"  // AttributeValueEquals checks if a specific attribute at the given path has a certain value.
	NullValue             ConditionType = "NullValue"             // NullValue verifies that value is "null"
	AttributeTypeIsNot ConditionType = "AttributeTypeIsNot" // AttributeTypeIsNot checks if a specific attribute at the given path is NOT of a certain cty.Type.
)

// ActionType is an enumeration defining the types of actions that can be performed by a Rule.
type ActionType string

const (
	RemoveAttribute   ActionType = "RemoveAttribute"   // RemoveAttribute removes a specific attribute at the given path.
	RemoveBlock       ActionType = "RemoveBlock"       // RemoveBlock removes a specific block at the given path.
	SetAttributeValue ActionType = "SetAttributeValue" // SetAttributeValue sets a specific attribute at the given path to a certain value.
	RemoveAllBlocksOfType ActionType = "RemoveAllBlocksOfType" // RemoveAllBlocksOfType removes all blocks of a specific type from the parent block.
	RemoveAllNestedBlocksMatchingPath ActionType = "RemoveAllNestedBlocksMatchingPath" // Removes all nested blocks matching a given path.
)

// RuleExecutionType defines how a rule should be executed.
type RuleExecutionType string

const (
	// RuleExecutionStandard indicates that the rule is applied to the resource block itself.
	RuleExecutionStandard RuleExecutionType = "Standard"
	// RuleExecutionForEachNestedBlock indicates that the rule is applied to each nested block of a specific type within the resource block.
	RuleExecutionForEachNestedBlock RuleExecutionType = "ForEachNestedBlock"
)

// RuleCondition defines a specific condition that must be met for a Rule's actions to be triggered.
// It specifies the type of check, the path to the HCL element, and an optional expected value.
type RuleCondition struct {
	Type ConditionType // Type is the kind of condition to check (e.g., AttributeExists, BlockExists).
	// Path is a slice of strings representing the hierarchical path to the attribute or block.
	// Example for a top-level attribute: `["attribute_name"]`
	// Example for a nested attribute: `["block_name", "nested_block_name", "attribute_name"]`
	// Example for a block: `["block_name", "nested_block_name"]`
	Path []string
	// Value is the cty.Value to compare against. This is used internally by the rule engine,
	// typically populated after parsing ExpectedValue, for the AttributeValueEquals condition type.
	Value cty.Value
	// ExpectedValue is the string representation of the value to compare against for AttributeValueEquals.
	// This string will be parsed into a cty.Value for comparison during rule processing.
	ExpectedValue string
	// ExpectedTypeFriendlyName is used with AttributeTypeIsNot to specify the type it should not be (e.g., "bool", "string", "number").
	// The engine will compare this against cty.Type.FriendlyName().
	ExpectedTypeFriendlyName string
}

// RuleAction defines an action to be performed on an HCL structure if all conditions of a Rule are met.
// It specifies the type of action, the path to the HCL element, and an optional value to set.
type RuleAction struct {
	Type ActionType // Type is the kind of action to perform (e.g., RemoveAttribute, SetAttributeValue).
	// Path is a slice of strings representing the hierarchical path to the attribute or block.
	// Example for a top-level attribute: `["attribute_name"]`
	// Example for a nested attribute: `["block_name", "nested_block_name", "attribute_name"]`
	// Example for removing a block: `["block_name", "nested_block_name"]`
	Path []string
	// ValueToSet is the string representation of the value to set for SetAttributeValue.
	// This string will be parsed into a cty.Value before the attribute is set.
	ValueToSet string
	// PathToSet is a slice of strings representing the hierarchical path to the attribute to set as Value.
	PathToSet []string
	// BlockTypeToRemove specifies the type of block to remove for the RemoveAllBlocksOfType action.
	BlockTypeToRemove string
}

// Rule defines a single, named modification operation to be conditionally applied to HCL resources.
// It consists of a target resource type, optional labels for more specific targeting, a set of
// conditions that must all be met, and a set of actions to perform if the conditions are true.
type Rule struct {
	Name string // Name is a human-readable identifier for the rule (e.g., "Remove_cluster_ipv4_cidr_when_ip_allocation_policy_exists").
	// TargetResourceType is the HCL resource type this rule applies to (e.g., "google_container_cluster").
	TargetResourceType string
	// TargetResourceLabels provide optional additional label criteria to narrow down the target resource.
	// For example, if TargetResourceType is "google_sql_database_instance", TargetResourceLabels could be ["my_db_instance_name"].
	// If empty, the rule applies to all resources of TargetResourceType.
	TargetResourceLabels []string
	Conditions           []RuleCondition // Conditions is a list of conditions that must ALL be true (AND logic) for the actions to be performed.
	Actions              []RuleAction    // Actions is a list of actions to be performed if all conditions are met.
	// ExecutionType specifies how the rule is executed. Defaults to RuleExecutionStandard.
	ExecutionType RuleExecutionType
	// NestedBlockTargetType is relevant when ExecutionType is RuleExecutionForEachNestedBlock.
	// It specifies the type of nested block to target (e.g., "node_pool").
	NestedBlockTargetType string
	// NestedBlockTargetLabels provide optional label criteria for the nested block.
	// If empty, the rule applies to all nested blocks of NestedBlockTargetType.
	NestedBlockTargetLabels []string
}
