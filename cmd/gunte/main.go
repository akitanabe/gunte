package main

import (
	"fmt"
	"io"
	"os"

	"github.com/akitanabe/gunte/internal/app"
	"github.com/akitanabe/gunte/internal/cli"
)

func main() {
	exitCode := run(
		os.Args[1:],
		os.Getwd,
		func(root string, request cli.ExecuteRequest) app.Result {
			return app.NewRunner(root).Execute(request)
		},
		os.Stdout,
		os.Stderr,
	)
	os.Exit(exitCode)
}

func run(args []string, getwd func() (string, error), execute func(string, cli.ExecuteRequest) app.Result, stdout, stderr io.Writer) int {
	parsed := cli.Parse(args)
	switch parsed.Kind {
	case cli.ParseHelp:
		fmt.Fprint(stdout, cli.Render(parsed.Topic))
		return app.ExitSuccess
	case cli.ParseUsageError:
		fmt.Fprintln(stderr, parsed.Message)
		return app.ExitUsage
	case cli.ParseExecute:
		root, err := getwd()
		if err != nil {
			fmt.Fprintln(stderr, "load_error: "+err.Error())
			return app.ExitFailure
		}
		result := execute(root, parsed.Request)
		writeDiagnostics(stderr, result)
		return result.ExitCode
	default:
		fmt.Fprintln(stderr, cli.UsageMessage)
		return app.ExitUsage
	}
}

func writeDiagnostics(stderr io.Writer, result app.Result) {
	for _, diagnostic := range result.Diagnostics {
		location := diagnostic.Path
		if diagnostic.Line > 0 {
			location = fmt.Sprintf("%s:%d:%d", diagnostic.Path, diagnostic.Line, diagnostic.Column)
		}
		if location == "" {
			location = diagnostic.ArtifactPath
		}
		if location == "" {
			fmt.Fprintf(stderr, "%s: %s\n", diagnostic.Kind, diagnostic.Message)
		} else {
			fmt.Fprintf(stderr, "%s: %s: %s\n", diagnostic.Kind, location, diagnostic.Message)
		}
		for _, related := range diagnostic.Related {
			fmt.Fprintf(stderr, "%s: related %s:%d:%d\n", diagnostic.Kind, related.Path, related.Line, related.Column)
		}
	}
}
