package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/hashicorp/hcl/v2" // Used by hclwrite and for hcl.Pos
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"
)

var logger *zap.Logger

func init() {
	var err error
	// Using NewDevelopment for more verbose output during development.
	// NewProduction() can be used for more structured, less verbose output in production.
	logger, err = zap.NewDevelopment()
	if err != nil {
		// Fallback to fmt.Fprintf if logger initialization fails.
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1) // Exit if logger cannot be initialized, as logging is critical.
	}
}

var rootCmd = &cobra.Command{
	Use:   "tf-modifier [file-path]",
	Short: "A CLI tool to modify Terraform files",
	Long:  `tf-modifier is a CLI tool that parses a Terraform (.tf) file, appends "-clone" to all "name" attributes, and saves the changes.`,
	Args:  cobra.ExactArgs(1), // Ensures exactly one argument (file path) is passed
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		logger.Info("Processing file", zap.String("filePath", filePath))

		// 1. Read the content of the file specified by the filePath argument.
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Error reading file", zap.String("filePath", filePath), zap.Error(err))
			return err // Return error to Cobra for handling
		}

		// 2. Parse into hclwrite.File using hclwrite.ParseConfig().
		hclFile, diags := hclwrite.ParseConfig(contentBytes, filePath, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			logger.Error("Error parsing HCL file", zap.String("filePath", filePath), zap.Stringer("diagnostics", diags))
			return fmt.Errorf("HCL parsing failed: %s", diags.Error())
		}

		modifiedCount := 0
		// 3. Traverse the parsed HCL body.
		for _, block := range hclFile.Body().Blocks() {
			nameAttr := block.Body().GetAttribute("name")
			if nameAttr == nil {
				continue
			}

			exprTokens := nameAttr.Expr().BuildTokens(nil)
			if len(exprTokens) == 0 {
				logger.Warn("Skipping 'name' attribute: no expression tokens found",
					zap.String("filePath", filePath),
					zap.String("blockType", block.Type()),
					zap.Strings("blockLabels", block.Labels()))
				continue
			}

			var originalStringValue string
			isSimpleQuotedString := false

			if len(exprTokens) == 1 && exprTokens[0].Type == hclwrite.TokenQuotedLit {
				tokenBytes := exprTokens[0].Bytes
				if len(tokenBytes) >= 2 && tokenBytes[0] == '"' && tokenBytes[len(tokenBytes)-1] == '"' {
					originalStringValue = string(tokenBytes[1 : len(tokenBytes)-1])
					isSimpleQuotedString = true
				} else {
					logger.Info("Skipping 'name' attribute: token is quoted but not a simple double-quoted string",
						zap.String("filePath", filePath),
						zap.String("blockType", block.Type()),
						zap.Strings("blockLabels", block.Labels()))
				}
			} else {
				logger.Info("Skipping 'name' attribute: not a simple quoted string",
					zap.String("filePath", filePath),
					zap.String("blockType", block.Type()),
					zap.Strings("blockLabels", block.Labels()),
					zap.Strings("tokenTypes", exprTokensToTypesHelper(exprTokens)))
			}

			if isSimpleQuotedString {
				modifiedStringValue := originalStringValue + "-clone"
				newTokens := hclwrite.TokensForValue(cty.StringVal(modifiedStringValue))
				block.Body().SetAttributeRaw("name", newTokens)
				logger.Info("Modified 'name' attribute",
					zap.String("filePath", filePath),
					zap.String("blockType", block.Type()),
					zap.Strings("blockLabels", block.Labels()),
					zap.String("from", originalStringValue),
					zap.String("to", modifiedStringValue))
				modifiedCount++
			}
		}

		if modifiedCount == 0 {
			logger.Info("No 'name' attributes were modified (or none were simple quoted strings)", zap.String("filePath", filePath))
		}

		modifiedBytes := hclFile.Bytes()
		err = os.WriteFile(filePath, modifiedBytes, 0644)
		if err != nil {
			logger.Error("Error writing modified HCL to file", zap.String("filePath", filePath), zap.Error(err))
			return err
		}

		logger.Info("Successfully modified and saved HCL file", zap.String("filePath", filePath))
		return nil
	},
}

// Helper function to get string representations of token types for logging.
func exprTokensToTypesHelper(tokens hclwrite.Tokens) []string {
	types := make([]string, 0, len(tokens))
	for _, t := range tokens {
		types = append(types, t.Type.String())
	}
	return types
}

func Execute() {
	// It's good practice to sync the logger before exiting.
	defer func() {
		if err := logger.Sync(); err != nil {
			// Log the sync error itself, if possible, or fallback to fmt.
			// This indicates a problem with the logging system itself.
			fmt.Fprintf(os.Stderr, "Error syncing logger: %v\n", err)
		}
	}()

	if err := rootCmd.Execute(); err != nil {
		// Errors from RunE should already be logged with context.
		// Cobra prints the error to os.Stderr by default.
		// We log a final message here before exiting with a non-zero status.
		logger.Fatal("Command execution failed", zap.Error(err))
		// os.Exit(1) is implicitly called by logger.Fatal, but can be explicit if not using Fatal.
		// If not using logger.Fatal, then:
		// logger.Error("Command execution failed", zap.Error(err))
		// os.Exit(1)
	}
}
