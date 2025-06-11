package main

import (
	"fmt"
	"os"

	"github.com/kotatut/cluster_import_cleaner/cmd"
	"go.uber.org/zap"
)

// main is the entry point of the application.
// It sets up a deferred panic recovery function and then executes the root command
// provided by the cmd package.
func main() {
	// Deferred function to recover from panics, log the error, and exit.
	defer func() {
		if r := recover(); r != nil {
			// Attempt to get the logger instance from the cmd package.
			// This logger is initialized within the cmd package (typically in root.go).
			currentLogger := cmd.GetCmdLogger()

			if currentLogger == nil {
				// Fallback to standard error output if the logger from cmd is not available
				// (e.g., panic occurred before logger initialization).
				// A simple zap logger is also created here as a last resort for structured logging of the panic.
				fmt.Fprintf(os.Stderr, "Panic recovery: Logger from cmd package not available. Panic: %v\n", r)

				simpleLogger, err := zap.NewDevelopment()
				if err == nil && simpleLogger != nil {
					simpleLogger.Error("Recovered from panic (fallback logger)",
						zap.String("panic_info", fmt.Sprintf("%v", r)),
						zap.Stack("stacktrace"), // Captures stack trace of the goroutine that panicked.
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
					zap.Stack("stacktrace"), // zap.Stack captures the stack trace.
				)
				_ = currentLogger.Sync() // Attempt to sync the logger from cmd.
			}
			os.Exit(1) // Exit with a non-zero status code after logging the panic.
		}
	}()

	// Execute the root command. The logger used by the application is initialized
	// within the cmd package (typically in cmd/root.go's init function).
	cmd.Execute()
}
