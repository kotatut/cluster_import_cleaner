package types

// ConditionType is an enumeration defining the types of conditions that can be checked by a Rule.
type ConditionType string

const (
	AttributeExists      ConditionType = "AttributeExists"
	AttributeDoesntExist ConditionType = "AttributeDoesntExist"
	BlockExists          ConditionType = "BlockExists"
	AttributeValueEquals ConditionType = "AttributeValueEquals"
	NullValue            ConditionType = "NullValue"
)

// ActionType is an enumeration defining the types of actions that can be performed by a Rule.
type ActionType string

const (
	RemoveAttribute                   ActionType = "RemoveAttribute"
	RemoveBlock                       ActionType = "RemoveBlock"
	SetAttributeValue                 ActionType = "SetAttributeValue"
	RemoveAllBlocksOfType             ActionType = "RemoveAllBlocksOfType"
	RemoveAllNestedBlocksMatchingPath ActionType = "RemoveAllNestedBlocksMatchingPath"
)

// RuleExecutionType defines how a rule should be executed.
type RuleExecutionType string

const (
	RuleExecutionStandard           RuleExecutionType = "Standard"
	RuleExecutionForEachNestedBlock RuleExecutionType = "ForEachNestedBlock"
)

// RuleCondition defines a specific condition that must be met for a Rule's actions to be triggered.
type RuleCondition struct {
	Type ConditionType
	// Path is a slice of strings representing the hierarchical path to the attribute or block.
	// Example for a top-level attribute: `["attribute_name"]`
	// Example for a nested attribute: `["block_name", "nested_block_name", "attribute_name"]`
	// Example for a block: `["block_name", "nested_block_name"]`
	Path []string
	// ExpectedValue is the string representation of the value to compare against for AttributeValueEquals.
	// This string will be parsed into a cty.Value for comparison during rule processing.
	ExpectedValue string
}

// RuleAction defines an action to be performed on an HCL structure if all conditions of a Rule are met.
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
type Rule struct {
	// Name is a human-readable identifier for the rule.
	Name string
	// TargetResourceType is the HCL resource type this rule applies to (e.g., "google_container_cluster").
	TargetResourceType string
	// Conditions is a list of conditions that must ALL be true.
	Conditions []RuleCondition
	// Actions is a list of actions to be performed if all conditions are met.
	Actions []RuleAction
	// ExecutionType specifies how the rule is executed. Defaults to RuleExecutionStandard.
	ExecutionType RuleExecutionType
	// It specifies the type of nested block to target (e.g., "node_pool").
	NestedBlockTargetType string
}
