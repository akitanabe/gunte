package main

import (
	"fmt"
	"os"

	"github.com/akitanabe/gunte/internal/app"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load_error: "+err.Error())
		os.Exit(app.ExitFailure)
	}
	result := app.NewRunner(root).Run(os.Args[1:])
	for _, diagnostic := range result.Diagnostics {
		location := diagnostic.Path
		if diagnostic.Line > 0 {
			location = fmt.Sprintf("%s:%d:%d", diagnostic.Path, diagnostic.Line, diagnostic.Column)
		}
		if location == "" {
			location = diagnostic.ArtifactPath
		}
		if location == "" {
			fmt.Fprintf(os.Stderr, "%s: %s\n", diagnostic.Kind, diagnostic.Message)
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s: %s\n", diagnostic.Kind, location, diagnostic.Message)
		}
		for _, related := range diagnostic.Related {
			fmt.Fprintf(os.Stderr, "%s: related %s:%d:%d\n", diagnostic.Kind, related.Path, related.Line, related.Column)
		}
	}
	os.Exit(result.ExitCode)
}
