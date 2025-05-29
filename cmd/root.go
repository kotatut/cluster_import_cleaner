package cmd

import (
	"fmt"
	"os"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier"
)

var logger *zap.Logger
var filePathVariable string // Variable to store the file path from the flag

func init() {
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Define the persistent --file flag
	rootCmd.PersistentFlags().StringVar(&filePathVariable, "file", "", "Path to the HCL file to modify (required)")
	// Mark the --file flag as required
	if err := rootCmd.MarkPersistentFlagRequired("file"); err != nil {
		// This error should ideally not happen for a newly defined flag.
		// If it does, it's a programming error in setting up Cobra.
		fmt.Fprintf(os.Stderr, "Error marking 'file' flag required: %v\n", err)
		os.Exit(1) // Exit if flag setup fails
	}
}

var rootCmd = &cobra.Command{
	Use:   "tf-modifier", // Removed [file-path] from Use, as it's now a flag
	Short: "A CLI tool to modify Terraform files",
	Long:  `tf-modifier is a CLI tool that parses a Terraform (.tf) file, appends "-clone" to all "name" attributes, and saves the changes.`,
	// Args: cobra.ExactArgs(1), // Removed positional argument validation
	RunE: func(cmd *cobra.Command, args []string) error {
		// filePath := args[0] // Removed: file path now comes from filePathVariable
		logger.Info("Processing file", zap.String("filePath", filePathVariable))

		// 1. Parse the HCL file using the hclmodifier package.
		// The logger from cmd/root.go is passed to the package function.
		hclFile, err := hclmodifier.NewFromFile(filePathVariable, logger)
		if err != nil {
			// ParseHCLFile already logs the detailed error.
			// We return the error to Cobra, which will typically print it to stderr.
			return fmt.Errorf("failed to parse HCL file: %w", err)
		}

		// 2. Modify the "name" attributes using the hclmodifier package.
		modifiedCount, err := hclFile.ModifyNameAttributes()
		if err != nil {
			// ModifyNameAttributes already logs the detailed error.
			return fmt.Errorf("failed to modify HCL attributes: %w", err)
		}
		logger.Info("Attribute modification complete", zap.Int("modifiedCount", modifiedCount), zap.String("filePath", filePathVariable))


		// 3. Write the modified HCL content back to the file using the hclmodifier package.
		err = hclFile.WriteToFile(filePathVariable)
		if err != nil {
			// WriteHCLFile already logs the detailed error.
			return fmt.Errorf("failed to write modified HCL file: %w", err)
		}

		logger.Info("Successfully processed and saved HCL file", zap.String("filePath", filePathVariable))
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
	// Changed logger.Fatal to logger.Error and added explicit os.Exit(1)
	logger.Error("Command execution failed", zap.Error(err))
	os.Exit(1)
	}
}

// GetCmdLogger returns the package-level logger instance.
// This is used by main.go for panic recovery logging.
func GetCmdLogger() *zap.Logger {
	return logger
}
