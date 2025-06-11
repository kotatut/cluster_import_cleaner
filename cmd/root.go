package cmd

import (
	"fmt"
	"os"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var logger *zap.Logger

// filePathFlag stores the path to the HCL file to be modified, provided via the --file flag.
var filePathFlag string

func init() {
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Define the persistent --file flag
	rootCmd.PersistentFlags().StringVar(&filePathFlag, "file", "", "Path to the HCL file to modify (required)")
	// Mark the --file flag as required
	if err := rootCmd.MarkPersistentFlagRequired("file"); err != nil {
		// This error should ideally not happen for a newly defined flag.
		// If it does, it's a programming error in setting up Cobra.
		fmt.Fprintf(os.Stderr, "Error marking 'file' flag required: %v\n", err)
		os.Exit(1) // Exit if flag setup fails
	}
}

// rootCmd represents the base command when called without any subcommands.
// It's configured to parse a GKE Terraform file, apply a series of cleaning rules,
// and save the modified file. The primary GKE-specific logic is encapsulated
// within the hclmodifier package and its rules.
var rootCmd = &cobra.Command{
	Use:   "gke-tf-cleaner",
	Short: "A CLI tool to clean and modify Terraform HCL files for GKE clusters.",
	Long: `gke-tf-cleaner is a command-line utility that processes a given Terraform HCL file.
It applies a predefined set of rules to clean up common issues found in configurations
for Google Kubernetes Engine (GKE) clusters, especially those generated from Terraform imports
or older templates. The tool modifies the file in-place.`,
	// Args: cobra.ExactArgs(1), // Positional argument for file path was replaced by a --file flag.
	RunE: func(cmd *cobra.Command, args []string) error {
		// filePath := args[0] // Removed: file path now comes from filePathFlag
		logger.Info("Processing file", zap.String("filePath", filePathFlag))

		// 1. Parse the HCL file using the hclmodifier package.
		// The logger from cmd/root.go is passed to the package function.
		hclFile, err := hclmodifier.NewFromFile(filePathFlag, logger)
		if err != nil {
			// NewFromFile already logs the detailed error.
			return fmt.Errorf("failed to parse HCL file: %w", err)
		}

		// 2. Define all rules to be applied by the generic ApplyRules engine.
		allRules := []hclmodifier.Rule{
			rules.ClusterIPV4CIDRRuleDefinition,
			rules.MasterCIDRRuleDefinition,
			rules.ServicesIPV4CIDRRuleDefinition,
			rules.BinaryAuthorizationRuleDefinition,
			rules.RuleRemoveLoggingService,
			rules.RuleRemoveMonitoringService,
			rules.RuleRemoveNodeVersion,
			rules.SetMinVersionRuleDefinition,
			// Note: InitialNodeCountRule and AutopilotRule are handled separately below
			// due to their complex logic not yet fully translated to the generic rule engine.
		}

		var encounteredErrors []error

		logger.Info("Applying generic rules...", zap.Int("ruleCount", len(allRules)))
		_, genericRuleErrors := hclFile.ApplyRules(allRules)
		if len(genericRuleErrors) > 0 {
			encounteredErrors = append(encounteredErrors, genericRuleErrors...)
		}
		// logger.Info("Generic rules application completed", zap.Int("totalModifications", modifications), zap.String("filePath", filePathFlag)) // Modifications count might be misleading if errors occurred

		// 3. Apply rules that have complex logic not yet fitting the generic engine.
		// Apply InitialNodeCount Rule (Complex: Iterates over sub-blocks 'node_pool')
		logger.Info("Applying InitialNodeCount Rule (custom logic)...")
		_, err = hclFile.ApplyInitialNodeCountRule()
		if err != nil {
			encounteredErrors = append(encounteredErrors, fmt.Errorf("InitialNodeCountRule failed: %w", err))
		}

		// Apply Autopilot Rule (Complex: Conditional logic based on attribute values, multiple different removals)
		logger.Info("Applying Autopilot Rule (custom logic)...")
		_, err = hclFile.ApplyAutopilotRule()
		if err != nil {
			encounteredErrors = append(encounteredErrors, fmt.Errorf("AutopilotRule failed: %w", err))
		}

		// 4. Write the modified HCL content back to the file.
		// This should happen regardless of rule application errors, as some rules might have succeeded.
		err = hclFile.WriteToFile(filePathFlag)
		if err != nil {
			// WriteHCLFile already logs the detailed error.
			return fmt.Errorf("failed to write modified HCL file: %w", err)
		}

		// 5. Report any errors encountered during rule processing.
		if len(encounteredErrors) > 0 {
			logger.Error("One or more rules encountered errors during processing file.", zap.String("filePath", filePathFlag))
			for _, ruleErr := range encounteredErrors {
				logger.Error("Rule application error", zap.Error(ruleErr))
			}
			return fmt.Errorf("encountered %d error(s) during rule processing on file %s. See logs for details", len(encounteredErrors), filePathFlag)
		}

		logger.Info("Successfully processed and saved HCL file", zap.String("filePath", filePathFlag))
		return nil
	},
}

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
