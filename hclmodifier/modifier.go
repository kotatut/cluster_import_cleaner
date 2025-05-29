package hclmodifier

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"
)

// ParseHCLFile reads a file from filePath, parses it as HCL, and returns an hclwrite.File.
// It uses the provided logger for logging any errors or diagnostic information.
func ParseHCLFile(filePath string, logger *zap.Logger) (*hclwrite.File, error) {
	if logger == nil {
		// Fallback if no logger is provided, though ideally, the caller should always provide one.
		logger, _ = zap.NewDevelopment() // Or zap.NewNop()
		logger.Warn("ParseHCLFile called with nil logger, using default development logger.")
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
		logger.Error("Error parsing HCL file", zap.String("filePath", filePath), zap.String("diagnostics", diags.Error()))
		return nil, fmt.Errorf("HCL parsing failed: %s", diags.Error())
	}

	return hclFile, nil
}

// WriteHCLFile writes the given hclwrite.File object to the specified filePath.
// It uses the provided logger for logging success or errors.
func WriteHCLFile(filePath string, file *hclwrite.File, logger *zap.Logger) error {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
		logger.Warn("WriteHCLFile called with nil logger, using default development logger.")
	}

	modifiedBytes := file.Bytes()
	logger.Debug("Writing modified HCL to file", zap.String("filePath", filePath))
	err := os.WriteFile(filePath, modifiedBytes, 0644)
	if err != nil {
		logger.Error("Error writing modified HCL to file", zap.String("filePath", filePath), zap.Error(err))
		return err
	}
	logger.Info("Successfully wrote modified HCL to file", zap.String("filePath", filePath))
	return nil
}

// ModifyNameAttributes iterates through the HCL file content and appends "-clone"
// to the value of any attribute named "name" that is a simple string literal.
// It uses GetAttribute, GetAttributeValue, and SetAttributeValue internally.
// It returns the count of modified attributes and an error if any unrecoverable issue occurs.
func ModifyNameAttributes(file *hclwrite.File, logger *zap.Logger) (int, error) {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
		logger.Warn("ModifyNameAttributes called with nil logger, using default development logger.")
	}

	modifiedCount := 0
	if file == nil || file.Body() == nil {
		logger.Error("ModifyNameAttributes called with nil file or file body.")
		return 0, fmt.Errorf("input file or file body cannot be nil")
	}

	for _, block := range file.Body().Blocks() {
		// Call GetAttribute to find the "name" attribute.
		nameAttribute, err := GetAttribute(block, "name", logger)
		if err != nil {
			// Attribute "name" not found in this block, or other error from GetAttribute.
			// Log this at debug level as it's expected for many blocks not to have a "name" attribute.
			logger.Debug("Attribute 'name' not found in block or error during GetAttribute",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()),
				zap.Error(err)) // Log the specific error from GetAttribute
			continue // Continue to the next block.
		}

		// If "name" attribute is found, get its value.
		attrValue, err := GetAttributeValue(nameAttribute, logger)
		if err != nil {
			// Error getting attribute value (e.g., not a simple literal, or other evaluation error).
			// Log this and continue. We only modify simple string literals.
			logger.Info("Skipping 'name' attribute: could not get simple literal value",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()),
				zap.String("attributeName", nameAttribute.Name()),
				zap.Error(err))
			continue
		}

		// Check if the value is a string.
		if attrValue.Type() == cty.String {
			originalStringValue := attrValue.AsString()
			modifiedStringValue := originalStringValue + "-clone"

			// Set the new string value.
			err = SetAttributeValue(block, "name", cty.StringVal(modifiedStringValue), logger)
			if err != nil {
				// Log the error from SetAttributeValue but consider if this should be a fatal error for the function.
				// For now, we'll log and continue, but this might indicate a problem.
				logger.Error("Failed to set modified 'name' attribute",
					zap.String("blockType", block.Type()),
					zap.Strings("blockLabels", block.Labels()),
					zap.String("originalValue", originalStringValue),
					zap.Error(err))
				// Depending on desired robustness, might want to return err here.
				// However, the original logic implies continuing, so we'll stick to that.
				continue
			}
			// Successfully set the new value
			logger.Info("Successfully modified 'name' attribute", // This log was in SetAttributeValue, but also good here.
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()),
				zap.String("from", originalStringValue),
				zap.String("to", modifiedStringValue))
			modifiedCount++
		} else {
			// Value is not a string, so we don't modify it.
			logger.Debug("Skipping 'name' attribute: value is not a string",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()),
				zap.String("attributeName", nameAttribute.Name()),
				zap.String("valueType", attrValue.Type().FriendlyName()))
		}
	}

	if modifiedCount == 0 {
		logger.Info("No 'name' attributes were modified (or none were suitable for modification).")
	} else {
		logger.Info("Total 'name' attributes modified", zap.Int("count", modifiedCount))
	}
	return modifiedCount, nil
}

// compareStringSlices checks if two string slices are identical in content and order.
func compareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GetBlock finds and returns a specific block from an HCL file based on its type and labels.
// If the block is not found, it returns nil and an error.
func GetBlock(hclFile *hclwrite.File, blockType string, blockLabels []string, logger *zap.Logger) (*hclwrite.Block, error) {
	if logger == nil {
		logger, _ = zap.NewDevelopment() // Fallback, though caller should provide.
		logger.Warn("GetBlock called with nil logger, using default development logger.")
	}

	logger.Debug("Searching for block",
		zap.String("blockType", blockType),
		zap.Strings("blockLabels", blockLabels))

	for _, block := range hclFile.Body().Blocks() {
		if block.Type() == blockType {
			if compareStringSlices(block.Labels(), blockLabels) {
				logger.Debug("Found matching block",
					zap.String("blockType", blockType),
					zap.Strings("blockLabels", blockLabels))
				return block, nil
			}
		}
	}

	logger.Warn("Block not found",
		zap.String("blockType", blockType),
		zap.Strings("blockLabels", blockLabels))
	return nil, fmt.Errorf("block %s %v not found", blockType, blockLabels)
}

// GetAttribute finds and returns a specific attribute from an HCL block by its name.
// If the attribute is not found, it returns nil and an error.
func GetAttribute(block *hclwrite.Block, attributeName string, logger *zap.Logger) (*hclwrite.Attribute, error) {
	if logger == nil {
		logger, _ = zap.NewDevelopment() // Fallback, though caller should provide.
		logger.Warn("GetAttribute called with nil logger, using default development logger.")
	}

	logger.Debug("Searching for attribute in block",
		zap.String("attributeName", attributeName),
		zap.String("blockType", block.Type()),       // Log block type for context
		zap.Strings("blockLabels", block.Labels())) // Log block labels for context

	// hclwrite.Body.GetAttribute directly retrieves the attribute by name if it exists.
	// There's no need to iterate manually if we're looking for a specific named attribute.
	attribute := block.Body().GetAttribute(attributeName)

	if attribute != nil {
		logger.Debug("Found attribute in block",
			zap.String("attributeName", attributeName),
			zap.String("blockType", block.Type()),
			zap.Strings("blockLabels", block.Labels()))
		return attribute, nil
	}

	logger.Warn("Attribute not found in block",
		zap.String("attributeName", attributeName),
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()))
	return nil, fmt.Errorf("attribute '%s' not found in block %s %v", attributeName, block.Type(), block.Labels())
}

// GetAttributeValue attempts to extract a cty.Value from an HCL attribute,
// focusing on simple literals (string, number, boolean) without interpolations or references.
// It uses a nil hcl.EvalContext, so expressions requiring context will result in an error.
func GetAttributeValue(attr *hclwrite.Attribute, logger *zap.Logger) (cty.Value, error) {
	if logger == nil {
		logger, _ = zap.NewDevelopment() // Fallback, though caller should provide.
		logger.Warn("GetAttributeValue called with nil logger, using default development logger.")
	}

	attrName := attr.Name()
	logger.Debug("Attempting to get value for attribute", zap.String("attributeName", attrName))

	if attr.Expr() == nil {
		logger.Warn("Attribute expression is nil", zap.String("attributeName", attrName))
		return cty.NilVal, fmt.Errorf("attribute '%s' has a nil expression", attrName)
	}

	// Attempt to evaluate the expression with a nil evaluation context.
	// This will work for true literals but fail for expressions requiring context (vars, functions).
	val, diags := attr.Expr().Value(nil)

	if diags.HasErrors() {
		logger.Warn("Diagnostics found while evaluating attribute value",
			zap.String("attributeName", attrName),
			zap.String("diagnostics", diags.Error()))

		// Check if diagnostics indicate a context-dependent expression (e.g., variable or function call)
		for _, diag := range diags {
			if diag.Subject == nil { // Some diags might not have a subject range
				continue
			}
			// These are common messages for undefined variables/functions when context is nil.
			// Note: hclsyntax. diagnóstico messages might vary. This is a best-effort check.
			// It might be more robust to check diag.Summary for keywords.
			if strings.Contains(diag.Summary, "Unknown variable") ||
				strings.Contains(diag.Summary, "Call to unknown function") ||
				strings.Contains(diag.Summary, "Unsupported attribute") { // e.g. trying to access attr of a non-object
				return cty.NilVal, fmt.Errorf("attribute '%s' is not a simple literal (likely a variable, function call, or unsupported reference): %w", attrName, diags)
			}
		}
		// If diagnostics are present but not clearly due to context dependency, treat as a general evaluation error.
		return cty.NilVal, fmt.Errorf("error evaluating attribute '%s' value: %w", attrName, diags)
	}

	// Check if the successfully evaluated value is of a supported simple literal type.
	if val.Type() == cty.String || val.Type() == cty.Number || val.Type() == cty.Bool {
		logger.Debug("Successfully extracted simple literal value for attribute",
			zap.String("attributeName", attrName),
			zap.String("valueType", val.Type().FriendlyName()),
			// zap.Any("value", val) can be too verbose, GoString is usually better for cty.Value
			zap.String("valueGoString", val.GoString()))
		return val, nil
	}

	// If the value is valid but not one of the supported primitive types.
	logger.Warn("Attribute value evaluated to an unsupported type for simple literal extraction",
		zap.String("attributeName", attrName),
		zap.String("valueType", val.Type().FriendlyName()))
	return cty.NilVal, fmt.Errorf("attribute '%s' evaluated to an unsupported type '%s' for simple literal extraction", attrName, val.Type().FriendlyName())
}

// SetAttributeValue sets an attribute on the given HCL block with the specified name and cty.Value.
// It will create a new attribute or overwrite an existing one.
func SetAttributeValue(block *hclwrite.Block, attributeName string, value cty.Value, logger *zap.Logger) error {
	if logger == nil {
		logger, _ = zap.NewDevelopment() // Fallback, though caller should provide.
		logger.Warn("SetAttributeValue called with nil logger, using default development logger.")
	}

	if block == nil {
		logger.Error("SetAttributeValue called with a nil block.")
		return fmt.Errorf("input block cannot be nil")
	}
	if block.Body() == nil {
		logger.Error("SetAttributeValue called with a block that has a nil body.", zap.String("blockType", block.Type()))
		return fmt.Errorf("input block %s has a nil body", block.Type())
	}


	logger.Debug("Attempting to set attribute value",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName),
		zap.String("valueType", value.Type().FriendlyName()),
		zap.String("valueGoString", value.GoString()))

	// Convert the cty.Value to HCL tokens.
	// TokensForValue is robust and handles various cty types, including null and unknown.
	// For null values, it will typically produce `null` tokens.
	// For unknown values, it might produce a representation that indicates an unknown value,
	// or it might panic if the value is truly problematic in a way it cannot serialize.
	// However, for typical usage with known, primitive values, this is safe.
	tokens := hclwrite.TokensForValue(value)

	// Set the attribute using the generated tokens.
	// SetAttributeRaw will create the attribute if it doesn't exist, or overwrite it if it does.
	block.Body().SetAttributeRaw(attributeName, tokens)

	logger.Info("Successfully set attribute",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName),
		zap.String("valueGoString", value.GoString()))

	return nil // SetAttributeRaw does not return an error.
}

// RemoveAttribute removes a specific attribute from an HCL block by its name.
// If the attribute does not exist, it logs this and returns nil (no error).
func RemoveAttribute(block *hclwrite.Block, attributeName string, logger *zap.Logger) error {
	if logger == nil {
		logger, _ = zap.NewDevelopment() // Fallback, though caller should provide.
		logger.Warn("RemoveAttribute called with nil logger, using default development logger.")
	}

	if block == nil {
		logger.Error("RemoveAttribute called with a nil block.")
		return fmt.Errorf("input block cannot be nil")
	}
	if block.Body() == nil {
		logger.Error("RemoveAttribute called with a block that has a nil body.", zap.String("blockType", block.Type()))
		return fmt.Errorf("input block %s has a nil body", block.Type())
	}

	logger.Debug("Attempting to remove attribute from block",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName))

	// Check if the attribute exists before attempting removal for more precise logging.
	existingAttr := block.Body().GetAttribute(attributeName)
	if existingAttr == nil {
		logger.Info("Attribute not found, no removal needed",
			zap.String("blockType", block.Type()),
			zap.Strings("blockLabels", block.Labels()),
			zap.String("attributeName", attributeName))
		return nil // Not an error if the attribute to be removed doesn't exist.
	}

	// Remove the attribute.
	block.Body().RemoveAttribute(attributeName)

	logger.Info("Successfully removed attribute",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName))

	return nil // RemoveAttribute does not return an error.
}

// RemoveBlock finds and removes a specific block from an HCL file based on its type and labels.
// If the block is not found, it logs this and returns an error.
func RemoveBlock(hclFile *hclwrite.File, blockType string, blockLabels []string, logger *zap.Logger) error {
	if logger == nil {
		logger, _ = zap.NewDevelopment() // Fallback, though caller should provide.
		logger.Warn("RemoveBlock called with nil logger, using default development logger.")
	}

	if hclFile == nil {
		logger.Error("RemoveBlock called with a nil hclFile.")
		return fmt.Errorf("input hclFile cannot be nil")
	}
	if hclFile.Body() == nil {
		logger.Error("RemoveBlock called with an hclFile that has a nil body.")
		return fmt.Errorf("input hclFile has a nil body")
	}

	logger.Debug("Attempting to remove block",
		zap.String("blockType", blockType),
		zap.Strings("blockLabels", blockLabels))

	var blockToRemove *hclwrite.Block
	var blockIndex int = -1

	// Iterate to find the block and its index.
	// While hclFile.Body().RemoveBlock(block *hclwrite.Block) exists,
	// we first need to find that block pointer.
	for i, block := range hclFile.Body().Blocks() {
		if block.Type() == blockType && compareStringSlices(block.Labels(), blockLabels) {
			blockToRemove = block
			blockIndex = i // Keep track of index just for logging or alternative removal, though RemoveBlock is preferred.
			break
		}
	}

	if blockToRemove != nil {
		// Use the direct RemoveBlock method.
		removed := hclFile.Body().RemoveBlock(blockToRemove)
		if !removed {
			// This case should theoretically not happen if blockToRemove was obtained from hclFile.Body().Blocks()
			// and no concurrent modifications are happening.
			logger.Error("Failed to remove block, RemoveBlock returned false",
				zap.String("blockType", blockType),
				zap.Strings("blockLabels", blockLabels),
				zap.Int("foundAtIndex", blockIndex)) // Log index for debugging
			return fmt.Errorf("failed to remove block %s %v, RemoveBlock method indicated failure", blockType, blockLabels)
		}

		logger.Info("Successfully removed block",
			zap.String("blockType", blockType),
			zap.Strings("blockLabels", blockLabels),
			zap.Int("foundAtIndex", blockIndex))
		return nil
	}

	logger.Warn("Block not found for removal",
		zap.String("blockType", blockType),
		zap.Strings("blockLabels", blockLabels))
	return fmt.Errorf("block %s %v not found for removal", blockType, blockLabels)
}


// exprTokensToTypesHelper converts HCL token types to strings for logging.
// This helper is kept private to the package.
func exprTokensToTypesHelper(tokens hclwrite.Tokens) []string {
	types := make([]string, 0, len(tokens))
	for _, t := range tokens {
		types = append(types, t.Type.String())
	}
	return types
}
