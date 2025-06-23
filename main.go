package main

import (
	"fmt"
	"os"

	"github.com/kotatut/cluster_import_cleaner/cmd"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Recovered from panic",
				zap.String("panic_info", fmt.Sprintf("%v", r)),
				zap.Stack("stacktrace"),
			)
			os.Exit(1)
		}
	}()

	cmd.Execute(logger)
}
