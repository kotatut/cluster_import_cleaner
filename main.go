package main

import (
	"fmt"
	"os"

	"github.com/kotatut/cluster_import_cleaner/cmd"
	"go.uber.org/zap"
)

// It sets up a deferred panic recovery function and then executes the root command
func main() {
	defer func() {
		if r := recover(); r != nil {
			// This logger is initialized within the cmd package (typically in root.go).
			currentLogger := cmd.GetCmdLogger()

			if currentLogger == nil {
				// Fallback to standard error output if the logger from cmd is not available
				fmt.Fprintf(os.Stderr, "Panic recovery: Logger from cmd package not available. Panic: %v\n", r)

				simpleLogger, err := zap.NewDevelopment()
				if err == nil && simpleLogger != nil {
					simpleLogger.Error("Recovered from panic (fallback logger)",
						zap.String("panic_info", fmt.Sprintf("%v", r)),
						zap.Stack("stacktrace"),
					)
					_ = simpleLogger.Sync()
				} else {
					// If even the simple logger fails, just print basic info.
					fmt.Fprintf(os.Stderr, "Recovered from panic: %v\nStack trace will be printed by Go runtime if not already.\n", r)
				}
			} else {
				// If the logger from cmd is available, use it to log the panic.
				currentLogger.Error("Recovered from panic",
					zap.String("panic_info", fmt.Sprintf("%v", r)),
					zap.Stack("stacktrace"),
				)
				_ = currentLogger.Sync()
			}
			os.Exit(1)
		}
	}()

	// Execute the root command. The logger used by the application is initialized
	cmd.Execute()
}
