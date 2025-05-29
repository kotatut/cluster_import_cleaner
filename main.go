package main

import (
	"fmt"
	"os"

	"github.com/kotatut/cluster_import_cleaner/cmd"
	"go.uber.org/zap" // Import zap for potential use in panic recovery
)

// Attempt to get the logger from the cmd package.
// This is a bit of a hack, as ideally, the logger would be passed around or accessible globally in a cleaner way.
// For panic recovery, direct access might be necessary if the panic occurs before logger is fully set up in Execute.
var globalLogger *zap.Logger

func main() {
	// Initialize globalLogger, trying to fetch from cmd.
	// This relies on cmd.GetLogger() or similar being available, or direct access if possible.
	// For this exercise, we'll assume cmd.InitializeLogger() sets up a logger that can be fetched,
	// or we directly try to use a logger from cmd if it's exported.
	// If cmd.Logger is not directly accessible, this part needs adjustment.
	// As per current cmd/root.go, logger is a package-level var, not easily accessible here without modification.
	// So, we'll initialize a temporary one here for panic recovery if cmd.Logger isn't available.

	defer func() {
		if r := recover(); r != nil {
			// Try to use the logger from cmd if it was initialized.
			// This is a simplification; a more robust solution would involve a global logger instance.
			// For now, we'll try to get it via a hypothetical GetLogger or use a default.
			currentLogger := cmd.GetCmdLogger() // Assuming GetCmdLogger() is added to cmd/root.go
			if currentLogger == nil {
				// Fallback if cmd's logger isn't available or not yet initialized
				fmt.Fprintf(os.Stderr, "Panic recovery: Logger not available. Panic: %v\n", r)
				// Attempt to create a simple logger for this panic message
				simpleLogger, _ := zap.NewDevelopment()
				if simpleLogger != nil {
					simpleLogger.Error("Recovered from panic",
						zap.String("panic_info", fmt.Sprintf("%v", r)),
						zap.Stack("stacktrace"),
					)
					_ = simpleLogger.Sync()
				} else {
					fmt.Fprintf(os.Stderr, "Recovered from panic: %v\nStack trace will be printed by Go runtime if not already.\n", r)
				}
			} else {
				currentLogger.Error("Recovered from panic",
					zap.String("panic_info", fmt.Sprintf("%v", r)),
					zap.Stack("stacktrace"), // zap.Stack automatically captures the stack trace for panics
				)
				_ = currentLogger.Sync() // Attempt to sync the main logger
			}
			os.Exit(1)
		}
	}()

	// It's good practice to initialize and pass the logger from main.
	// For now, cmd.Execute() initializes its own logger.
	cmd.Execute()
}
