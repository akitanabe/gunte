package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/app"
	"github.com/akitanabe/gunte/internal/cli"
)

func TestHelpDoesNotGetWorkingDirectoryOrExecuteProject(t *testing.T) {
	var getwdCalls, executeCalls int
	var stdout, stderr bytes.Buffer
	code := run(
		[]string{"--help"},
		func() (string, error) {
			getwdCalls++
			return "", errors.New("getwd must not be called")
		},
		func(string, cli.ExecuteRequest) app.Result {
			executeCalls++
			return app.Result{ExitCode: app.ExitFailure}
		},
		&stdout,
		&stderr,
	)
	if code != app.ExitSuccess || getwdCalls != 0 || executeCalls != 0 {
		t.Fatalf("help code=%d getwd=%d execute=%d", code, getwdCalls, executeCalls)
	}
	if stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatalf("help stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestUsageErrorDoesNotGetWorkingDirectoryOrExecuteProject(t *testing.T) {
	var getwdCalls, executeCalls int
	var stdout, stderr bytes.Buffer
	code := run(
		[]string{"help", "emit", "extra"},
		func() (string, error) {
			getwdCalls++
			return "", nil
		},
		func(string, cli.ExecuteRequest) app.Result {
			executeCalls++
			return app.Result{ExitCode: app.ExitSuccess}
		},
		&stdout,
		&stderr,
	)
	if code != app.ExitUsage || getwdCalls != 0 || executeCalls != 0 {
		t.Fatalf("usage code=%d getwd=%d execute=%d", code, getwdCalls, executeCalls)
	}
	if stdout.Len() != 0 || stderr.String() != cli.UsageMessage+"\n" {
		t.Fatalf("usage stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestBinaryHelpEntrypointsUseStdoutAndExitSuccessfullyOutsideProject(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
	bin := filepath.Join(t.TempDir(), "gunte")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/gunte")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	cases := []struct {
		args   []string
		marker string
	}{
		{args: []string{"--help"}, marker: "gunte <command> [options]"},
		{args: []string{"-h"}, marker: "gunte <command> [options]"},
		{args: []string{"help"}, marker: "gunte <command> [options]"},
		{args: []string{"help", "emit"}, marker: "Usage: gunte emit [--target ID]"},
		{args: []string{"help", "check"}, marker: "Usage: gunte check [--target ID]"},
		{args: []string{"help", "lock"}, marker: "Usage: gunte lock"},
		{args: []string{"emit", "--help"}, marker: "Usage: gunte emit [--target ID]"},
		{args: []string{"check", "--help"}, marker: "Usage: gunte check [--target ID]"},
		{args: []string{"lock", "--help"}, marker: "Usage: gunte lock"},
	}
	for _, test := range cases {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			command := exec.Command(bin, test.args...)
			command.Dir = t.TempDir()
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					t.Fatalf("exit=%d stdout=%q stderr=%q", exitError.ExitCode(), stdout.String(), stderr.String())
				}
				t.Fatal(err)
			}
			if command.ProcessState.ExitCode() != app.ExitSuccess || !strings.Contains(stdout.String(), test.marker) || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", command.ProcessState.ExitCode(), stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunExecutesValidRequestOnceAndPropagatesExitCode(t *testing.T) {
	var getwdCalls, executeCalls int
	var gotRoot string
	var gotRequest cli.ExecuteRequest
	var stdout, stderr bytes.Buffer
	wantResult := app.Result{ExitCode: app.ExitFailure}
	code := run(
		[]string{"emit", "--target", "claude"},
		func() (string, error) {
			getwdCalls++
			return "/project", nil
		},
		func(root string, request cli.ExecuteRequest) app.Result {
			executeCalls++
			gotRoot = root
			gotRequest = request
			return wantResult
		},
		&stdout,
		&stderr,
	)
	if code != app.ExitFailure || getwdCalls != 1 || executeCalls != 1 {
		t.Fatalf("code=%d getwd=%d execute=%d", code, getwdCalls, executeCalls)
	}
	if gotRoot != "/project" || gotRequest != (cli.ExecuteRequest{Command: cli.CommandEmit, Target: "claude"}) {
		t.Fatalf("root=%q request=%#v", gotRoot, gotRequest)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestOccurrenceDiagnosticWritesCountsToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	actual, expected := int64(1), int64(2)
	code := run(
		[]string{"emit"},
		func() (string, error) { return "/project", nil },
		func(string, cli.ExecuteRequest) app.Result {
			return app.Result{ExitCode: app.ExitFailure, Diagnostics: []app.Diagnostic{{Kind: "occurrences_violation", Path: "contracts.toml", Line: 4, Column: 1, ArtifactPath: "out/a.md", ActualCount: &actual, ExpectedCount: &expected, Message: "predicate count failed for target one: expected count 2, actual count 1"}}}
		},
		&stdout,
		&stderr,
	)
	if code != app.ExitFailure || stdout.Len() != 0 || !strings.Contains(stderr.String(), "expected count 2, actual count 1") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestBinaryExecuteFailureUsesStderrAndExitOne(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
	bin := filepath.Join(t.TempDir(), "gunte")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/gunte")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	command := exec.Command(bin, "emit")
	command.Dir = t.TempDir()
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != app.ExitFailure || stdout.Len() != 0 || !strings.Contains(stderr.String(), "load_error:") {
		t.Fatalf("error=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}

func TestBinaryUsageErrorsUseStderrAndExitTwo(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
	bin := filepath.Join(t.TempDir(), "gunte")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/gunte")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	command := exec.Command(bin, "help", "emit", "extra")
	command.Dir = t.TempDir()
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err == nil {
		t.Fatalf("usage unexpectedly succeeded stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != app.ExitUsage || stdout.Len() != 0 || stderr.String() != cli.UsageMessage+"\n" {
		t.Fatalf("usage exit=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(command.Dir, "gunte.toml")); !os.IsNotExist(err) {
		t.Fatalf("unexpected project file in non-project cwd: %v", err)
	}
}
