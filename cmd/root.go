package cmd

import (
	"fmt"
	"os"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier"
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

		// 1. Parse the HCL file using the hclmodifier package.
		// The logger from cmd/root.go is passed to the package function.
		hclFile, err := modifier.NewFromFile(filePath, logger)
		if err != nil {
			// ParseHCLFile already logs the detailed error.
			// We return the error to Cobra, which will typically print it to stderr.
			return fmt.Errorf("failed to parse HCL file: %w", err)
		}

		// 2. Modify the "name" attributes using the hclmodifier package.
		modifiedCount, err := modifier.ModifyNameAttributes()
		if err != nil {
			// ModifyNameAttributes already logs the detailed error.
			return fmt.Errorf("failed to modify HCL attributes: %w", err)
		}
		logger.Info("Attribute modification complete", zap.Int("modifiedCount", modifiedCount), zap.String("filePath", filePath))


		// 3. Write the modified HCL content back to the file using the hclmodifier package.
		err = modifier.WriteToFile(filePath, hclFile, logger)
		if err != nil {
			// WriteHCLFile already logs the detailed error.
			return fmt.Errorf("failed to write modified HCL file: %w", err)
		}

		logger.Info("Successfully processed and saved HCL file", zap.String("filePath", filePath))
		return nil
	},
}

// exprTokensToTypesHelper is no longer needed here as it's in hclmodifier package (if still public, or private there).

func Execute() {
	// It's good practice to sync the logger before exiting.
	defer func() {
		if errSync := logger.Sync(); errSync != nil {
			// Log the sync error itself, if possible, or fallback to fmt.
			// This indicates a problem with the logging system itself.
			// Using fmt.Fprintf directly as logger.Sync() itself failed.
			fmt.Fprintf(os.Stderr, "Error syncing logger: %v\n", errSync)
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
