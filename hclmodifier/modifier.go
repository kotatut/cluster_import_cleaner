package hclmodifier

import (
	"fmt"
	"os"
	"slices"
	"strconv" // Added for parsing string to bool/number

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types" // Import the new types package
)

// Modifier encapsulates an HCL file that can be programmatically modified.
// It holds the parsed HCL file representation and a logger for operational insights.
type Modifier struct {
	file   *hclwrite.File // The in-memory representation of the HCL file.
	Logger *zap.Logger    // Logger for logging activities within the modifier.
}

// NewFromFile creates a new Modifier instance by reading and parsing an HCL file
// from the specified filePath. It initializes a development logger if none is provided.
// filePath: Path to the HCL file to be read and parsed.
// logger: A zap.Logger instance. If nil, a new development logger is created.
// Returns a pointer to the created Modifier and an error if file reading or parsing fails.
func NewFromFile(filePath string, logger *zap.Logger) (*Modifier, error) {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
		logger.Warn("NewFromFile called with nil logger, using default development logger.")
	}

	logger.Debug("Reading HCL file", zap.String("filePath", filePath))
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("Error reading file", zap.String("filePath", filePath), zap.Error(err))
		return nil, err
	}

	logger.Debug("Parsing HCL file", zap.String("filePath", filePath))
	hclFile, diags := hclwrite.ParseConfig(contentBytes, filePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		logger.Error("Error parsing HCL file", zap.String("filePath", filePath), zap.Error(diags))
		return nil, fmt.Errorf("HCL parsing failed: %w", diags)
	}

	return &Modifier{file: hclFile, Logger: logger}, nil
}

// File provides access to the underlying *hclwrite.File object that the Modifier is working with.
// This allows for direct inspection or manipulation of the HCL file structure if necessary,
// though most common operations should be covered by Modifier's methods.
// Returns the *hclwrite.File associated with this Modifier.
func (m *Modifier) File() *hclwrite.File {
	return m.file
}

// WriteToFile serializes the current state of the Modifier's hclwrite.File object
// back into HCL format and writes it to the specified filePath.
// filePath: The path where the modified HCL content should be saved.
// Returns an error if writing to the file fails.
func (m *Modifier) WriteToFile(filePath string) error {
	modifiedBytes := m.file.Bytes()
	m.Logger.Debug("Writing modified HCL to file", zap.String("filePath", filePath))
	err := os.WriteFile(filePath, modifiedBytes, 0644)
	if err != nil {
		m.Logger.Error("Error writing modified HCL to file", zap.String("filePath", filePath), zap.Error(err))
		return fmt.Errorf("failed to write HCL content to %s: %w", filePath, err)
	}
	m.Logger.Info("Successfully wrote modified HCL to file", zap.String("filePath", filePath))
	return nil
}

// GetBlock searches for and returns a specific HCL block within the Modifier's file
// based on its type and a complete list of labels.
// blockType: The type of the block to find (e.g., "resource", "data", "module").
// blockLabels: A slice of strings representing all labels of the block in order (e.g., ["google_container_cluster", "my_cluster"]).
// Returns the found *hclwrite.Block and nil error if successful, or nil and an error if not found.
func (m *Modifier) GetBlock(blockType string, blockLabels []string) (*hclwrite.Block, error) {
	m.Logger.Debug("Searching for block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	for _, block := range m.file.Body().Blocks() {
		if block.Type() == blockType && slices.Equal(block.Labels(), blockLabels) {
			m.Logger.Debug("Found matching block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
			return block, nil
		}
	}
	m.Logger.Warn("Block not found", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	return nil, fmt.Errorf("block %s %v not found", blockType, blockLabels)
}

// GetAttribute retrieves a specific attribute by its name from the provided HCL block.
// block: The *hclwrite.Block from which to retrieve the attribute.
// attributeName: The name of the attribute to find.
// Returns the found *hclwrite.Attribute and nil error if successful, or nil and an error if not found.
func (m *Modifier) GetAttribute(block *hclwrite.Block, attributeName string) (*hclwrite.Attribute, error) {
	attribute := block.Body().GetAttribute(attributeName)
	if attribute == nil {
		m.Logger.Debug("Attribute not found in block",
			zap.String("attributeName", attributeName),
			zap.String("blockType", block.Type()),
			zap.Strings("blockLabels", block.Labels()))
		return nil, fmt.Errorf("attribute '%s' not found", attributeName)
	}
	return attribute, nil
}

// GetAttributeValue evaluates the expression of an HCL attribute and returns its corresponding cty.Value.
// This method is primarily intended for attributes with literal values (strings, numbers, booleans),
// as it uses a nil hcl.EvalContext, meaning it cannot resolve variables or function calls.
// It serves as a bridge between hclwrite's syntactic representation and hcl's value evaluation.
// attr: The *hclwrite.Attribute whose expression is to be evaluated.
// Returns the evaluated cty.Value and nil error if successful. If parsing or evaluation fails
// (e.g., the expression is not a simple literal), it returns cty.NilVal and an error.
func (m *Modifier) GetAttributeValue(attr *hclwrite.Attribute) (cty.Value, error) {
	// 1. Get the source bytes of the expression from the hclwrite attribute.
	exprBytes := attr.Expr().BuildTokens(nil).Bytes()

	// 2. Parse these bytes into an evaluatable hcl.Expression using the hclsyntax package.
	expr, diags := hclsyntax.ParseExpression(exprBytes, "attribute_expr", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		m.Logger.Error("Failed to re-parse attribute expression for evaluation.", zap.Error(diags))
		return cty.NilVal, fmt.Errorf("failed to parse expression: %w", diags)
	}

	// 3. Now, with an hcl.Expression, we can call .Value() to get the cty.Value.
	// We pass a nil EvalContext because we only want to resolve simple literals.
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		m.Logger.Debug("Attribute expression is not a simple literal", zap.String("expression", string(exprBytes)), zap.Error(diags))
		return cty.NilVal, fmt.Errorf("attribute is not a simple literal: %w", diags)
	}

	return val, nil
}

// SetAttributeValue sets or replaces an attribute within the specified HCL block.
// block: The *hclwrite.Block where the attribute should be set.
// attributeName: The name of the attribute to set.
// value: The cty.Value to assign to the attribute. This value will be converted to its HCL representation.
// Returns an error if the input block or its body is nil.
func (m *Modifier) SetAttributeValue(block *hclwrite.Block, attributeName string, value cty.Value) error {
	if block == nil || block.Body() == nil {
		return fmt.Errorf("input block or its body cannot be nil")
	}
	block.Body().SetAttributeValue(attributeName, value)
	m.Logger.Debug("Successfully set attribute",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName))
	return nil
}

// RemoveAttribute deletes a specific attribute from the provided HCL block by its name.
// If the attribute does not exist in the block, the operation is a no-op and returns nil.
// block: The *hclwrite.Block from which to remove the attribute.
// attributeName: The name of the attribute to remove.
// Returns an error if the input block or its body is nil.
func (m *Modifier) RemoveAttribute(block *hclwrite.Block, attributeName string) error {
	if block == nil || block.Body() == nil {
		return fmt.Errorf("input block or its body cannot be nil")
	}
	if block.Body().GetAttribute(attributeName) == nil {
		m.Logger.Debug("Attribute to remove not found, no action needed.", zap.String("attributeName", attributeName))
		return nil
	}
	block.Body().RemoveAttribute(attributeName)
	m.Logger.Debug("Successfully removed attribute",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName))
	return nil
}

// RemoveBlock finds a specific block by its type and labels, and then removes it from the HCL file body.
// blockType: The type of the block to remove (e.g., "resource").
// blockLabels: A slice of strings representing all labels of the block to remove.
// Returns an error if the block is not found or if the removal fails.
func (m *Modifier) RemoveBlock(blockType string, blockLabels []string) error {
	blockToRemove, err := m.GetBlock(blockType, blockLabels)
	if err != nil {
		return fmt.Errorf("cannot remove block that was not found: %w", err)
	}
	if removed := m.file.Body().RemoveBlock(blockToRemove); !removed {
		m.Logger.Error("Failed to remove block, RemoveBlock method returned false", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
		return fmt.Errorf("failed to remove block %s %v", blockType, blockLabels)
	}
	m.Logger.Info("Successfully removed block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	return nil
}

// GetNestedBlock navigates through a sequence of HCL block names (path) starting from currentBlockBody
// to find and return a specific nested block.
// currentBlockBody: The *hclwrite.Body of the block from which to start the search.
// path: A slice of strings where each string is a block type/name in the nesting hierarchy.
// For example, to find block "c" in `a { b { c {} } }`, path would be `["a", "b", "c"]` if starting from root,
// or `["b", "c"]` if `currentBlockBody` is the body of block `a`.
// Returns the found *hclwrite.Block and nil error, or nil and an error if any block in the path is not found.
func (m *Modifier) GetNestedBlock(currentBlockBody *hclwrite.Body, path []string) (*hclwrite.Block, error) {
	if currentBlockBody == nil {
		return nil, fmt.Errorf("GetNestedBlock: currentBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return nil, fmt.Errorf("GetNestedBlock: path cannot be empty")
	}

	logger := m.Logger.With(zap.Strings("path", path))
	logger.Debug("GetNestedBlock: Attempting to find nested block.")

	var currentLevelBody = currentBlockBody
	var foundBlock *hclwrite.Block

	for i, blockName := range path {
		foundBlock = nil // Reset for current level
		for _, block := range currentLevelBody.Blocks() {
			if block.Type() == blockName {
				if i == len(path)-1 {
					foundBlock = block
					break
				}
				currentLevelBody = block.Body()
				foundBlock = block
				break
			}
		}
		if foundBlock == nil {
			logger.Debug("GetNestedBlock: Block not found at current level.", zap.String("blockName", blockName), zap.Int("level", i))
			return nil, fmt.Errorf("block '%s' not found at path level %d", blockName, i)
		}
	}

	if foundBlock == nil {
		logger.Debug("GetNestedBlock: Target block not found at the end of path.")
		return nil, fmt.Errorf("target block not found at path '%s'", path)
	}

	logger.Debug("GetNestedBlock: Successfully found nested block.")
	return foundBlock, nil
}

// GetAttributeValueByPath retrieves the cty.Value and the *hclwrite.Attribute for an attribute
// specified by a path, starting from an initialBlockBody. The path can point to an attribute
// directly within initialBlockBody or within a deeply nested block.
// initialBlockBody: The *hclwrite.Body to begin the search from.
// path: A slice of strings representing the path. The last element is the attribute name,
// and preceding elements are nested block names. E.g., `["parent_block", "child_block", "attribute_name"]`.
// For a direct attribute in initialBlockBody, path is `["attribute_name"]`.
// Returns the cty.Value of the attribute, the *hclwrite.Attribute itself, and an error if the
// path is invalid, any intermediate block is not found, the attribute is not found, or its value cannot be determined.
func (m *Modifier) GetAttributeValueByPath(initialBlockBody *hclwrite.Body, path []string) (cty.Value, *hclwrite.Attribute, error) {
	if initialBlockBody == nil {
		return cty.NilVal, nil, fmt.Errorf("GetAttributeValueByPath: initialBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return cty.NilVal, nil, fmt.Errorf("GetAttributeValueByPath: path cannot be empty")
	}

	logger := m.Logger.With(zap.Strings("path", path))
	logger.Debug("GetAttributeValueByPath: Attempting to get attribute value.")

	attributeName := path[len(path)-1]
	blockPath := path[:len(path)-1]

	var targetBody *hclwrite.Body
	if len(blockPath) == 0 {
		targetBody = initialBlockBody
	} else {
		parentBlock, err := m.GetNestedBlock(initialBlockBody, blockPath)
		if err != nil {
			logger.Error("GetAttributeValueByPath: Could not find parent block for attribute.", zap.Error(err))
			return cty.NilVal, nil, fmt.Errorf("parent block for attribute '%s' not found: %w", attributeName, err)
		}
		if parentBlock.Body() == nil {
			logger.Error("GetAttributeValueByPath: Parent block has no body.", zap.Strings("blockPath", blockPath))
			return cty.NilVal, nil, fmt.Errorf("parent block '%s' has no body", blockPath)
		}
		targetBody = parentBlock.Body()
	}

	attr := targetBody.GetAttribute(attributeName)
	if attr == nil {
		logger.Debug("GetAttributeValueByPath: Attribute not found.", zap.String("attributeName", attributeName))
		return cty.NilVal, nil, fmt.Errorf("attribute '%s' not found in specified block", attributeName)
	}

	val, err := m.GetAttributeValue(attr)
	if err != nil {
		logger.Debug("GetAttributeValueByPath: Could not get value of attribute.", zap.String("attributeName", attributeName), zap.Error(err))
		return cty.NilVal, attr, fmt.Errorf("could not get value of attribute '%s': %w", attributeName, err)
	}

	logger.Debug("GetAttributeValueByPath: Successfully retrieved attribute value.", zap.String("attributeName", attributeName))
	return val, attr, nil
}

// RemoveAttributeByPath removes an attribute specified by a path, starting from an initialBlockBody.
// The path can point to an attribute directly within initialBlockBody or within a deeply nested block.
// initialBlockBody: The *hclwrite.Body to begin the search from.
// path: A slice of strings representing the path. The last element is the attribute name to remove,
// and preceding elements are nested block names. E.g., `["parent_block", "child_block", "attribute_name"]`.
// For a direct attribute in initialBlockBody, path is `["attribute_name"]`.
// Returns an error if the path is invalid or any intermediate block is not found.
// If the attribute to be removed does not exist at the specified path, it's a no-op and returns nil.
func (m *Modifier) RemoveAttributeByPath(initialBlockBody *hclwrite.Body, path []string) error {
	if initialBlockBody == nil {
		return fmt.Errorf("RemoveAttributeByPath: initialBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return fmt.Errorf("RemoveAttributeByPath: path cannot be empty")
	}

	logger := m.Logger.With(zap.Strings("path", path))
	logger.Debug("RemoveAttributeByPath: Attempting to remove attribute.")

	attributeName := path[len(path)-1]
	blockPath := path[:len(path)-1]

	var targetBody *hclwrite.Body
	if len(blockPath) == 0 {
		targetBody = initialBlockBody
	} else {
		parentBlock, err := m.GetNestedBlock(initialBlockBody, blockPath)
		if err != nil {
			logger.Error("RemoveAttributeByPath: Could not find parent block for attribute.", zap.Error(err))
			return fmt.Errorf("parent block for attribute '%s' not found: %w", attributeName, err)
		}
		if parentBlock.Body() == nil {
			logger.Error("RemoveAttributeByPath: Parent block has no body.", zap.Strings("blockPath", blockPath))
			return fmt.Errorf("parent block '%s' has no body", blockPath)
		}
		targetBody = parentBlock.Body()
	}

	if targetBody.GetAttribute(attributeName) == nil {
		logger.Debug("RemoveAttributeByPath: Attribute to remove not found, no action needed.", zap.String("attributeName", attributeName))
		return nil
	}

	targetBody.RemoveAttribute(attributeName)
	logger.Info("RemoveAttributeByPath: Successfully removed attribute.", zap.String("attributeName", attributeName))
	return nil
}

// RemoveNestedBlockByPath removes a nested block specified by a path, starting from an initialBlockBody.
// initialBlockBody: The *hclwrite.Body of the block from which the removal path starts.
// path: A slice of strings where each string is a block type/name in the nesting hierarchy,
// leading to the block to be removed. The last element in the path is the name of the block to remove.
// E.g., to remove block "c" in `a { b { c {} } }`, path would be `["b", "c"]` if `initialBlockBody` is body of `a`.
// Returns an error if the path is invalid or any intermediate parent block is not found.
// If the block to be removed does not exist at the specified path, it's a no-op and returns nil.
func (m *Modifier) RemoveNestedBlockByPath(initialBlockBody *hclwrite.Body, path []string) error {
	if initialBlockBody == nil {
		return fmt.Errorf("RemoveNestedBlockByPath: initialBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return fmt.Errorf("RemoveNestedBlockByPath: path cannot be empty")
	}

	logger := m.Logger.With(zap.Strings("path", path))
	logger.Debug("RemoveNestedBlockByPath: Attempting to remove nested block.")

	blockToRemoveName := path[len(path)-1]
	parentBlockPath := path[:len(path)-1]

	var bodyToRemoveFrom *hclwrite.Body
	if len(parentBlockPath) == 0 {
		bodyToRemoveFrom = initialBlockBody
	} else {
		parentBlock, err := m.GetNestedBlock(initialBlockBody, parentBlockPath)
		if err != nil {
			logger.Error("RemoveNestedBlockByPath: Could not find parent block of the block to remove.", zap.Error(err))
			return fmt.Errorf("parent block for '%s' not found: %w", blockToRemoveName, err)
		}
		if parentBlock.Body() == nil {
			logger.Error("RemoveNestedBlockByPath: Parent block has no body.", zap.Strings("parentBlockPath", parentBlockPath))
			return fmt.Errorf("parent block '%s' has no body", parentBlockPath)
		}
		bodyToRemoveFrom = parentBlock.Body()
	}

	var blockToRemove *hclwrite.Block
	for _, block := range bodyToRemoveFrom.Blocks() {
		if block.Type() == blockToRemoveName {
			blockToRemove = block
			break
		}
	}

	if blockToRemove == nil {
		logger.Debug("RemoveNestedBlockByPath: Block to remove not found, no action needed.", zap.String("blockToRemoveName", blockToRemoveName))
		return nil
	}

	if removed := bodyToRemoveFrom.RemoveBlock(blockToRemove); !removed {
		logger.Error("RemoveNestedBlockByPath: Failed to remove block using RemoveBlock method.", zap.String("blockToRemoveName", blockToRemoveName))
		return fmt.Errorf("failed to remove block '%s'", blockToRemoveName)
	}

	logger.Info("RemoveNestedBlockByPath: Successfully removed nested block.", zap.String("blockToRemoveName", blockToRemoveName))
	return nil
}

// ApplyRules processes a slice of Rule definitions and applies them to the Modifier's HCL file.
// It iterates through each rule and, for each rule, through the resource blocks in the HCL file.
//
// Rule Execution Types:
// - RuleExecutionStandard: Conditions and actions are applied directly to the main resource block
//   that matches TargetResourceType and TargetResourceLabels. Paths in conditions/actions are
//   relative to this resource block's body.
// - RuleExecutionForEachNestedBlock: After finding a matching main resource block, this rule
//   iterates through all its direct sub-blocks. If a sub-block's type matches the rule's
//   NestedBlockTargetType (and optionally NestedBlockTargetLabels), the conditions and actions
//   are applied to that sub-block. Paths in conditions/actions are then relative to this
//   nested sub-block's body.
//
// The function accumulates the total number of successful modifications and a list of any errors
// encountered. Processing continues even if some rules or actions result in errors.
//
// inputRules: A slice of Rule structs to be applied.
// Returns:
//   modifications: The total number of successful modifications made to the HCL file.
//   errors: A slice of errors encountered during the application of any rule.
func (m *Modifier) ApplyRules(inputRules []types.Rule) (modifications int, errors []error) {
	m.Logger.Info("Starting ApplyRules processing.", zap.Int("numberOfRules", len(inputRules)))
	totalModifications := 0
	var collectedErrors []error

	if m.file == nil || m.file.Body() == nil {
		m.Logger.Error("ApplyRules: Modifier's file or file body is nil.")
		collectedErrors = append(collectedErrors, fmt.Errorf("modifier's file or file body cannot be nil"))
		return 0, collectedErrors
	}

	for _, currentRule := range inputRules { // Use currentRule from iteration over inputRules
		ruleLogger := m.Logger.With(zap.String("ruleName", currentRule.Name), zap.String("targetResourceType", currentRule.TargetResourceType))
		ruleLogger.Debug("Processing rule.")

		// Initialize ExecutionType if it's empty (for backward compatibility or default behavior)
		if currentRule.ExecutionType == "" {
			currentRule.ExecutionType = types.RuleExecutionStandard
		}
		ruleLogger = ruleLogger.With(zap.String("executionType", string(currentRule.ExecutionType)))

		for _, resourceBlock := range m.file.Body().Blocks() {
			if resourceBlock.Type() != "resource" || len(resourceBlock.Labels()) == 0 || resourceBlock.Labels()[0] != currentRule.TargetResourceType {
				continue
			}

			if len(currentRule.TargetResourceLabels) > 0 {
				if len(resourceBlock.Labels()) < 1+len(currentRule.TargetResourceLabels) {
					continue
				}
				match := true
				for i, expectedLabel := range currentRule.TargetResourceLabels {
					if resourceBlock.Labels()[i+1] != expectedLabel {
						match = false
						break
					}
				}
				if !match {
					continue
				}
			}
			resourceLogger := ruleLogger.With(zap.Strings("resourceLabels", resourceBlock.Labels()))
			resourceLogger.Debug("Target resource matched.")

			// Standard execution: conditions and actions apply to the resourceBlock itself.
			// Paths for conditions/actions are relative to the resourceBlock's body.
			if currentRule.ExecutionType == types.RuleExecutionStandard {
				resourceLogger.Debug("Executing as Standard Rule. Checking conditions for the resource block itself.")
				conditionsMet := true
				for _, condition := range currentRule.Conditions {
					condLogger := resourceLogger.With(zap.String("conditionType", string(condition.Type)), zap.Strings("conditionPath", condition.Path))
					// Conditions are checked against the main resource block's body.
					if !m.checkCondition(resourceBlock.Body(), condition, condLogger) {
						conditionsMet = false
						break
					}
				}

				if conditionsMet {
					resourceLogger.Info("All conditions met for resource. Performing actions on the resource block.")
					for _, action := range currentRule.Actions {
						actLogger := resourceLogger.With(zap.String("actionType", string(action.Type)), zap.Strings("actionPath", action.Path))
						// Actions are performed on the main resource block's body.
						errAction := m.performAction(resourceBlock.Body(), action, actLogger, currentRule.Name, resourceBlock.Labels())
						if errAction == nil {
							totalModifications++
						} else {
							collectedErrors = append(collectedErrors, errAction)
						}
					}
				} else {
					resourceLogger.Debug("Not all conditions met for resource block.")
				}
			// ForEachNestedBlock execution: conditions and actions apply to each matching nested block
			// within the resourceBlock. Paths are relative to the nested block's body.
			} else if currentRule.ExecutionType == types.RuleExecutionForEachNestedBlock {
				resourceLogger.Debug("Executing as ForEachNestedBlock Rule.", zap.String("nestedBlockTargetType", currentRule.NestedBlockTargetType))
				if currentRule.NestedBlockTargetType == "" {
					resourceLogger.Warn("NestedBlockTargetType is not defined for ForEachNestedBlock rule. Skipping this rule for this resource.", zap.String("ruleName", currentRule.Name))
					collectedErrors = append(collectedErrors, fmt.Errorf("rule '%s' is ForEachNestedBlock but NestedBlockTargetType is empty", currentRule.Name))
					continue // Skip to the next resource block or rule
				}

				// Iterate over direct sub-blocks of the matched resource block.
				for _, nestedBlock := range resourceBlock.Body().Blocks() {
					if nestedBlock.Type() == currentRule.NestedBlockTargetType {
						// TODO: Implement label matching for nestedBlock if currentRule.NestedBlockTargetLabels is populated.
						// For now, it matches any nested block of the specified type.
						nestedBlockLogger := resourceLogger.With(zap.String("nestedBlockType", nestedBlock.Type()), zap.Strings("nestedBlockLabels", nestedBlock.Labels()))
						nestedBlockLogger.Debug("Matching nested block found. Checking conditions for this nested block.")

						conditionsMet := true
						for _, condition := range currentRule.Conditions {
							condLogger := nestedBlockLogger.With(zap.String("conditionType", string(condition.Type)), zap.Strings("conditionPath", condition.Path))
							// Conditions are checked against the nested block's body.
							// Paths in 'condition.Path' are relative to this 'nestedBlock.Body()'.
							if !m.checkCondition(nestedBlock.Body(), condition, condLogger) {
								conditionsMet = false
								break
							}
						}

						if conditionsMet {
							nestedBlockLogger.Info("All conditions met for nested block. Performing actions on this nested block.")
							for _, action := range currentRule.Actions {
								actLogger := nestedBlockLogger.With(zap.String("actionType", string(action.Type)), zap.Strings("actionPath", action.Path))
								// Actions are performed on the nested block's body.
								// Paths in 'action.Path' are relative to this 'nestedBlock.Body()'.
								errAction := m.performAction(nestedBlock.Body(), action, actLogger, currentRule.Name, resourceBlock.Labels())
								if errAction == nil {
									totalModifications++
								} else {
									collectedErrors = append(collectedErrors, errAction)
								}
							}
						} else {
							nestedBlockLogger.Debug("Not all conditions met for this nested block.")
						}
					}
				}
			}
		}
	}

	m.Logger.Info("ApplyRules processing finished.", zap.Int("totalModifications", totalModifications), zap.Int("numberOfErrors", len(collectedErrors)))
	if len(collectedErrors) > 0 {
		for _, e := range collectedErrors {
			m.Logger.Error("ApplyRules encountered an error during processing.", zap.Error(e))
		}
		return totalModifications, collectedErrors
	}
	return totalModifications, nil
}

// checkCondition evaluates a single RuleCondition against a given hclwrite.Body.
// This function is a helper for ApplyRules, used for both standard and nested block execution types.
//
// initialBlockBody: The *hclwrite.Body against which the condition is evaluated. For standard rules,
//                   this is the body of the main resource. For RuleExecutionForEachNestedBlock,
//                   this is the body of the current nested block being processed.
// condition: The RuleCondition to check. Paths within the condition are relative to initialBlockBody.
// condLogger: A zap.Logger instance pre-configured with context for this condition check.
// Returns true if the condition is met, false otherwise.
func (m *Modifier) checkCondition(initialBlockBody *hclwrite.Body, condition types.RuleCondition, condLogger *zap.Logger) bool {
	switch condition.Type {
	case types.AttributeExists:
		// Checks if an attribute at condition.Path exists within initialBlockBody.
		_, _, err := m.GetAttributeValueByPath(initialBlockBody, condition.Path)
		if err != nil {
			condLogger.Debug("Condition AttributeExists not met (attribute not found or error accessing).", zap.Error(err))
			return false
		}
	case types.AttributeDoesntExists:
		// Checks if an attribute at condition.Path does NOT exist within initialBlockBody.
		_, _, err := m.GetAttributeValueByPath(initialBlockBody, condition.Path)
		if err == nil { // If attribute exists (no error getting it), condition is not met.
			condLogger.Debug("Condition AttributeDoesntExists not met (attribute was found).")
			return false
		}
		// If err is not nil, it means attribute wasn't found or path was invalid, so condition is met.
		// Specific error checking could be done here if needed (e.g., distinguish "not found" from "invalid path").
	case types.BlockExists:
		// Checks if a nested block at condition.Path exists within initialBlockBody.
		_, err := m.GetNestedBlock(initialBlockBody, condition.Path)
		if err != nil {
			condLogger.Debug("Condition BlockExists not met (block not found or error accessing).", zap.Error(err))
			return false
		}
	case types.NullValue:
		// Checks if an attribute at condition.Path exists and its value is cty.Null.
		val, _, err := m.GetAttributeValueByPath(initialBlockBody, condition.Path)
		if err != nil || !val.IsNull() { // Error getting value, or value is not null.
			condLogger.Debug("Condition NullValue not met.", zap.Error(err), zap.Bool("isNull", val.IsNull()))
			return false
		}
	case types.AttributeValueEquals:
		// Checks if an attribute at condition.Path exists and its value equals condition.ExpectedValue.
		// Comparison logic attempts to parse ExpectedValue based on the actual attribute's type.
		val, _, err := m.GetAttributeValueByPath(initialBlockBody, condition.Path)
		if err != nil {
			condLogger.Debug("AttributeValueEquals: Attribute not found for comparison.", zap.Error(err))
			return false
		}

		var expectedCtyValue cty.Value
		var parseErr error

		// Attempt to parse ExpectedValue based on the actual value's type
		switch val.Type() {
		case cty.String:
			expectedCtyValue = cty.StringVal(condition.ExpectedValue)
		case cty.Bool:
			boolVal, errConv := strconv.ParseBool(condition.ExpectedValue)
			if errConv != nil {
				parseErr = fmt.Errorf("failed to parse ExpectedValue '%s' as bool: %w", condition.ExpectedValue, errConv)
			} else {
				expectedCtyValue = cty.BoolVal(boolVal)
			}
		case cty.Number:
			if intVal, errConv := strconv.ParseInt(condition.ExpectedValue, 10, 64); errConv == nil {
				expectedCtyValue = cty.NumberIntVal(intVal)
			} else if floatVal, errConv := strconv.ParseFloat(condition.ExpectedValue, 64); errConv == nil {
				expectedCtyValue = cty.NumberFloatVal(floatVal)
			} else {
				parseErr = fmt.Errorf("failed to parse ExpectedValue '%s' as number: %v or %v", condition.ExpectedValue, errConv, errConv)
			}
		default:
			condLogger.Warn("AttributeValueEquals: Actual value type is not a primitive type supported for robust ExpectedValue parsing. Condition will likely not be met.", zap.Any("actualValueType", val.Type()))
			return false // Cannot reliably compare non-primitive types with a string ExpectedValue
		}

		if parseErr != nil {
			condLogger.Warn("AttributeValueEquals: Error parsing ExpectedValue, condition not met.", zap.Error(parseErr), zap.String("expectedStr", condition.ExpectedValue), zap.Any("actualType", val.Type()))
			return false
		}
		if !val.Equals(expectedCtyValue).True() {
			condLogger.Debug("AttributeValueEquals not met.", zap.Any("actualValue", val.GoString()), zap.Any("parsedExpectedValue", expectedCtyValue.GoString()))
			return false
		}
	default:
		condLogger.Warn("Unknown condition type.")
		return false
	}
	return true
}

// performAction executes a single RuleAction on a given hclwrite.Body.
// This function is a helper for ApplyRules, used for both standard and nested block execution types.
//
// initialBlockBody: The *hclwrite.Body on which the action is performed. For standard rules,
//                   this is the body of the main resource. For RuleExecutionForEachNestedBlock,
//                   this is the body of the current nested block being processed.
// action: The RuleAction to perform. Paths within the action are relative to initialBlockBody.
// actLogger: A zap.Logger instance pre-configured with context for this action.
// ruleName: The name of the rule whose action is being performed (for error reporting).
// resourceLabels: The labels of the main resource block being processed (for error reporting).
// Returns an error if the action failed, nil otherwise (indicating a modification was made).
func (m *Modifier) performAction(initialBlockBody *hclwrite.Body, action types.RuleAction, actLogger *zap.Logger, ruleName string, resourceLabels []string) error {
	var errAction error
	switch action.Type {
	case types.RemoveAttribute:
		// Removes an attribute at action.Path within initialBlockBody.
		errAction = m.RemoveAttributeByPath(initialBlockBody, action.Path)
		if errAction == nil {
			actLogger.Info("Action RemoveAttribute successful.")
		}
	case types.RemoveBlock:
		// Removes a nested block at action.Path within initialBlockBody.
		errAction = m.RemoveNestedBlockByPath(initialBlockBody, action.Path)
		if errAction == nil {
			actLogger.Info("Action RemoveBlock successful.")
		}
	case types.SetAttributeValue:
		// Sets an attribute at action.Path within initialBlockBody to a specified value.
		// The value can be derived from action.ValueToSet (parsed string) or action.PathToSet (another attribute's value).
		var valueToSet cty.Value
		if len(action.PathToSet) != 0 {
			// Get value from another attribute (PathToSet) within the same scope (initialBlockBody)
			valueByPath, _, err := m.GetAttributeValueByPath(initialBlockBody, action.PathToSet)
			if err != nil {
				errAction = fmt.Errorf("error getting value from PathToSet '%v': %w", action.PathToSet, err)
			} else {
				valueToSet = valueByPath
			}
		} else { // Parse ValueToSet string into cty.Value
			// This is a simplified parser; a more robust solution might be needed for complex types or interpolations
			if bVal, err := strconv.ParseBool(action.ValueToSet); err == nil {
				valueToSet = cty.BoolVal(bVal)
			} else if iVal, err := strconv.ParseInt(action.ValueToSet, 10, 64); err == nil {
				valueToSet = cty.NumberIntVal(iVal)
			} else if fVal, err := strconv.ParseFloat(action.ValueToSet, 64); err == nil {
				valueToSet = cty.NumberFloatVal(fVal)
			} else {
				// Default to string if not parsable as bool or number.
				// This might need refinement if explicit type hints are available or needed.
				valueToSet = cty.StringVal(action.ValueToSet)
			}
		}

		if errAction == nil { // Only proceed if PathToSet was successful (or not used)
			errAction = m.SetAttributeValueByPath(initialBlockBody, action.Path, valueToSet)
			if errAction == nil {
				actLogger.Info("Action SetAttributeValue successful.")
			}
		}
	default:
		actLogger.Warn("Unknown action type.")
		errAction = fmt.Errorf("unknown action type: %s", action.Type)
	}

	if errAction != nil {
		actLogger.Error("Error performing action.", zap.Error(errAction))
		return fmt.Errorf("rule '%s' action '%s' on resource '%s' (or its sub-block) failed: %w", ruleName, action.Type, resourceLabels, errAction)
	}
	return nil
}

// SetAttributeValueByPath sets an attribute at a potentially nested path within an initialBlockBody.
// The path can point to an attribute directly within initialBlockBody or within a deeply nested block.
// initialBlockBody: The *hclwrite.Body to begin the operation from.
// path: A slice of strings representing the path. The last element is the attribute name to set,
//       and preceding elements are nested block names. E.g., `["parent_block", "attribute_name"]`.
// valueToSet: The cty.Value to set for the attribute.
// Returns an error if the path is invalid, any intermediate block is not found, or if the
// initialBlockBody itself is nil.
func (m *Modifier) SetAttributeValueByPath(initialBlockBody *hclwrite.Body, path []string, valueToSet cty.Value) error {
	if initialBlockBody == nil {
		return fmt.Errorf("SetAttributeValueByPath: initialBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return fmt.Errorf("SetAttributeValueByPath: path cannot be empty")
	}

	logger := m.Logger.With(zap.Strings("path", path), zap.Any("valueToSet", valueToSet.GoString()))
	logger.Debug("SetAttributeValueByPath: Attempting to set attribute value.")

	attributeName := path[len(path)-1]
	blockPath := path[:len(path)-1]

	var targetBody *hclwrite.Body
	if len(blockPath) == 0 {
		targetBody = initialBlockBody
	} else {
		parentBlock, err := m.GetNestedBlock(initialBlockBody, blockPath)
		if err != nil {
			logger.Error("SetAttributeValueByPath: Could not find parent block for attribute.", zap.Error(err))
			return fmt.Errorf("parent block for attribute '%s' not found: %w", attributeName, err)
		}
		if parentBlock.Body() == nil {
			logger.Error("SetAttributeValueByPath: Parent block has no body.", zap.Strings("blockPath", blockPath))
			return fmt.Errorf("parent block '%s' has no body", blockPath)
		}
		targetBody = parentBlock.Body()
	}

	targetBody.SetAttributeValue(attributeName, valueToSet)
	logger.Info("SetAttributeValueByPath: Successfully set attribute.", zap.String("attributeName", attributeName))
	return nil
}
