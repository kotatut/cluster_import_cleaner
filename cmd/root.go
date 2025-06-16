package cmd

import (
	"fmt"
	"os"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var logger *zap.Logger

var filePathFlag string

func init() {
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// --file flag as required
	rootCmd.PersistentFlags().StringVar(&filePathFlag, "file", "", "Path to the HCL file to modify (required)")
	rootCmd.MarkPersistentFlagRequired("file")
}

// Configure command to parse a GKE Terraform file, apply a series of cleaning rules, and save the modified file.
var rootCmd = &cobra.Command{
	Use:   "gke-tf-cleaner",
	Short: "A CLI tool to clean and modify Terraform HCL files for GKE clusters.",
	Long: `gke-tf-cleaner is a command-line utility that processes a given Terraform HCL file.
It applies a predefined set of rules to clean up common issues found in configurations
for Google Kubernetes Engine (GKE) clusters, especially those generated from Terraform imports
or older templates. The tool modifies the file in-place.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.Info("Processing file", zap.String("filePath", filePathFlag))
		hclFile, err := hclmodifier.NewFromFile(filePathFlag, logger)
		if err != nil {
			return fmt.Errorf("failed to parse HCL file: %w", err)
		}

		// Define all rules to be applied by the generic ApplyRules engine.
		allRules := []types.Rule{
			rules.ClusterIPV4CIDRRuleDefinition,
			rules.MasterCIDRRuleDefinition,
			rules.ServicesIPV4CIDRRuleDefinition,
			rules.PodIPV4CIDRRuleDefinition,
			rules.BinaryAuthorizationRuleDefinition,
			rules.RuleRemoveLoggingService,
			rules.RemoveLoggingServiceOnConfigPresentRule,
			rules.RuleRemoveMonitoringService,
			rules.SetMinVersionRuleDefinition,
			rules.HpaProfileRuleDefinition,
			rules.DiskSizeRuleDefinition,
			rules.OsVersionRuleDefinition,
			rules.OsVersionNodePoolRuleDefinition,
			rules.InitialNodeCountRuleDefinition,
			rules.RuleHandleAutopilotFalse,
			rules.RuleTerraformLabel,
		}
		allRules = append(allRules, rules.AutopilotRules...)
		allRules = append(allRules, rules.TopLevelComputedAttributesRules...)
		allRules = append(allRules, rules.OtherComputedAttributesRules...)

		var encounteredErrors []error

		logger.Info("Applying generic rules...", zap.Int("ruleCount", len(allRules)))
		modifications, genericRuleErrors := hclFile.ApplyRules(allRules)
		if len(genericRuleErrors) > 0 {
			encounteredErrors = append(encounteredErrors, genericRuleErrors...)
		}
		logger.Info("Generic rules application completed", zap.Int("totalModifications", modifications), zap.String("filePath", filePathFlag))

		// Write the modified HCL content back to the file.
		// This should happen regardless of rule application errors, as some rules might have succeeded.
		err = hclFile.WriteToFile(filePathFlag)
		if err != nil {
			return fmt.Errorf("failed to write modified HCL file: %w", err)
		}

		// Report any errors encountered during rule processing.
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
			fmt.Fprintf(os.Stderr, "Error syncing logger: %v\n", errSync)
		}
	}()

	if err := rootCmd.Execute(); err != nil {
		// Cobra prints the error to os.Stderr by default.
		// We log a final message here before exiting with a non-zero status.
		logger.Error("Command execution failed", zap.Error(err))
		os.Exit(1)
	}
}

// This is used by main.go for panic recovery logging.
func GetCmdLogger() *zap.Logger {
	return logger
}
