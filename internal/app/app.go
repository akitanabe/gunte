// Package app wires the pure build calculations to project-root filesystem
// actions. It deliberately keeps loading and writing at the boundary so that
// failed calculations can be proven not to reach the writer.
package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/akitanabe/gunte/internal/adapter"
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/contract"
	"github.com/akitanabe/gunte/internal/lexer"
	"github.com/akitanabe/gunte/internal/serialize"
	"github.com/akitanabe/gunte/internal/source"
)

const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Diagnostic is the stable CLI result. Paths are project-relative; Path is the
// primary path and falls back to ArtifactPath when a contract predicate has no
// path. ArtifactPath identifies the affected output artifact, Related preserves
// source locations in order, and Line/Column use one-origin coordinates.
type Diagnostic struct {
	Kind         string
	Path         string
	Line         int
	Column       int
	ArtifactPath string
	Related      []Location
	Message      string
}

// Location identifies a project-relative source position with one-origin line
// and column coordinates.
type Location struct {
	Path   string
	Line   int
	Column int
}

// Result contains the exit status and deterministic diagnostics of one run.
type Result struct {
	ExitCode    int
	Diagnostics []Diagnostic
}

type reader interface {
	ReadFile(path string) ([]byte, error)
}

type writer interface {
	Write(path string, data []byte) error
}

// Runner executes one emit or check command for a fixed project root.
type Runner struct {
	root   string
	reader reader
	writer writer
}

// NewRunner creates a runner backed by the local filesystem.
func NewRunner(root string) Runner {
	return Runner{root: root, reader: localReader{}, writer: localWriter{root: root}}
}

func newRunnerWithWriter(root string, output writer) Runner {
	return Runner{root: root, reader: localReader{}, writer: output}
}

func newRunnerWithReader(root string, input reader, output writer) Runner {
	return Runner{root: root, reader: input, writer: output}
}

// Run parses the minimal CLI grammar and executes emit or check.
func (runner Runner) Run(args []string) Result {
	command, selected, diagnostic := parseArgs(args)
	if diagnostic != nil {
		return Result{ExitCode: ExitUsage, Diagnostics: []Diagnostic{*diagnostic}}
	}
	project, diagnostics := runner.loadProject()
	if len(diagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: diagnostics}
	}
	if selected != "" && !containsTarget(project, selected) {
		return Result{ExitCode: ExitFailure, Diagnostics: []Diagnostic{{Kind: "unknown_target", Path: "gunte.toml", Message: "unknown target " + selected}}}
	}
	registry, diagnostics := runner.loadRegistry(project)
	if len(diagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: diagnostics}
	}
	units, diagnostics := runner.loadSources(project)
	if len(diagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: diagnostics}
	}
	projection, sourceDiagnostics := compile.ValidateAndProject(project, units)
	if len(sourceDiagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: sourceDiagnosticResults(sourceDiagnostics)}
	}
	artifacts, diagnostics := runner.generate(project, projection, units, selected)
	if len(diagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: diagnostics}
	}
	contractResult, contractDiagnostics := contract.EvaluateTargets(registry, artifacts, selectedIDs(selected))
	if len(contractDiagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: contractDiagnosticResults(contractDiagnostics)}
	}
	if len(contractResult.Violations) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: violationResults(contractResult.Violations)}
	}
	if command == "check" {
		return runner.check(artifacts)
	}
	return runner.emit(artifacts)
}

func parseArgs(args []string) (string, string, *Diagnostic) {
	usage := func(message string) (string, string, *Diagnostic) {
		return "", "", &Diagnostic{Kind: "usage", Message: message}
	}
	if len(args) < 1 || (args[0] != "emit" && args[0] != "check") {
		return usage("usage: gunte emit|check [--target ID]")
	}
	if len(args) == 1 {
		return args[0], "", nil
	}
	if len(args) != 3 || args[1] != "--target" || args[2] == "" {
		return usage("usage: gunte emit|check [--target ID]")
	}
	return args[0], args[2], nil
}

func (runner Runner) loadProject() (config.ProjectConfig, []Diagnostic) {
	data, err := runner.read("gunte.toml")
	if err != nil {
		return config.ProjectConfig{}, []Diagnostic{{Kind: "load_error", Path: "gunte.toml", Message: err.Error()}}
	}
	project, configDiagnostics := config.ParseProject("gunte.toml", data)
	return project, configDiagnosticResults(configDiagnostics)
}

func (runner Runner) loadRegistry(project config.ProjectConfig) (config.ContractRegistry, []Diagnostic) {
	data, err := runner.read("contracts.toml")
	if err != nil {
		return config.ContractRegistry{}, []Diagnostic{{Kind: "load_error", Path: "contracts.toml", Message: err.Error()}}
	}
	registry, configDiagnostics := config.ParseContracts("contracts.toml", data, project.TargetIDs())
	return registry, configDiagnosticResults(configDiagnostics)
}

func (runner Runner) loadSources(project config.ProjectConfig) ([]compile.SourceUnit, []Diagnostic) {
	units := make([]compile.SourceUnit, 0, len(project.Sources.Files))
	var diagnostics []Diagnostic
	for _, path := range project.Sources.Files {
		data, err := runner.read(path)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Kind: "load_error", Path: path, Message: err.Error()})
			continue
		}
		document, sourceDiagnostics := source.Parse(path, data)
		if len(sourceDiagnostics) != 0 {
			diagnostics = append(diagnostics, sourceDiagnosticResults(sourceDiagnostics)...)
			continue
		}
		ir, lexerDiagnostics := lexer.Lex(path, document.Buffer, document.BodyRange)
		if len(lexerDiagnostics) != 0 {
			diagnostics = append(diagnostics, sourceDiagnosticResults(lexerDiagnostics)...)
			continue
		}
		units = append(units, compile.SourceUnit{Path: path, Document: document, IR: ir})
	}
	return units, diagnostics
}

func (runner Runner) generate(project config.ProjectConfig, projection compile.Result, units []compile.SourceUnit, selected string) ([]serialize.Artifact, []Diagnostic) {
	var artifacts []serialize.Artifact
	var diagnostics []Diagnostic
	for targetIndex, target := range project.Targets {
		if selected != "" && target.ID != selected {
			continue
		}
		if targetIndex >= len(projection.Targets) {
			return nil, []Diagnostic{{Kind: "compile_error", Path: target.ID, Message: "missing target projection"}}
		}
		targetProject := project
		targetProject.Targets = []config.Target{target}
		targetSources := make([]adapter.Source, len(units))
		for sourceIndex, unit := range units {
			targetSources[sourceIndex] = adapter.Source{Projection: projection.Targets[targetIndex].Sources[sourceIndex], Frontmatter: unit.Document.FrontmatterData}
		}
		adapted, adapterDiagnostics := adapter.Adapt(targetProject, targetSources)
		for _, diagnostic := range adapterDiagnostics {
			if diagnostic.Severity == adapter.SeverityError {
				diagnostics = append(diagnostics, Diagnostic{Kind: diagnostic.Code, Path: diagnostic.Source, Message: diagnostic.Message})
			}
		}
		if len(diagnostics) != 0 {
			return nil, diagnostics
		}
		for _, logical := range adapted.Artifacts {
			serialized, serializeDiagnostics := serialize.Serialize(logical)
			if len(serializeDiagnostics) != 0 {
				for _, diagnostic := range serializeDiagnostics {
					diagnostics = append(diagnostics, Diagnostic{Kind: diagnostic.Code, Path: diagnostic.Path, Message: diagnostic.Message})
				}
				return nil, diagnostics
			}
			artifacts = append(artifacts, serialized)
		}
	}
	return artifacts, nil
}

func (runner Runner) emit(artifacts []serialize.Artifact) Result {
	result := Result{ExitCode: ExitSuccess}
	for _, artifact := range artifacts {
		if err := runner.writer.Write(artifact.Path, artifact.Bytes); err != nil {
			result.ExitCode = ExitFailure
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Kind: "write_error", Path: artifact.Path, Message: err.Error()})
			return result
		}
	}
	return result
}

func (runner Runner) check(artifacts []serialize.Artifact) Result {
	result := Result{ExitCode: ExitSuccess}
	for _, artifact := range artifacts {
		data, err := runner.read(artifact.Path)
		if diagnostic, failed := compareOutput(outputComparison{Path: artifact.Path, Expected: artifact.Bytes, Actual: data, Err: err}); failed {
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
	}
	if len(result.Diagnostics) != 0 {
		result.ExitCode = ExitFailure
	}
	return result
}

func (runner Runner) read(relative string) ([]byte, error) {
	return runner.reader.ReadFile(filepath.Join(runner.root, filepath.FromSlash(relative)))
}

func containsTarget(project config.ProjectConfig, id string) bool {
	for _, target := range project.Targets {
		if target.ID == id {
			return true
		}
	}
	return false
}

func selectedIDs(selected string) []string {
	if selected == "" {
		return nil
	}
	return []string{selected}
}

func configDiagnosticResults(diagnostics []config.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, len(diagnostics))
	for index, diagnostic := range diagnostics {
		result[index] = Diagnostic{Kind: "config_error", Path: diagnostic.Path, Line: diagnostic.Line, Column: diagnostic.Column, Message: diagnostic.Message}
	}
	return result
}

func sourceDiagnosticResults(diagnostics []source.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, len(diagnostics))
	for index, diagnostic := range diagnostics {
		result[index] = Diagnostic{Kind: "source_error", Path: diagnostic.Path, Line: diagnostic.Line, Column: diagnostic.Column, Message: diagnostic.Message}
	}
	return result
}

func contractDiagnosticResults(diagnostics []contract.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, len(diagnostics))
	for index, diagnostic := range diagnostics {
		path := diagnostic.Predicate.Path
		line, column := diagnostic.Predicate.Line, diagnostic.Predicate.Column
		if path == "" {
			path = diagnostic.ArtifactPath
		}
		result[index] = Diagnostic{Kind: string(diagnostic.Kind), Path: path, Line: line, Column: column, ArtifactPath: diagnostic.ArtifactPath, Related: locations(diagnostic.RelatedSource), Message: diagnostic.Message}
	}
	return result
}

func violationResults(violations []contract.Violation) []Diagnostic {
	result := make([]Diagnostic, len(violations))
	for index, violation := range violations {
		result[index] = Diagnostic{Kind: string(violation.Kind), Path: violation.Predicate.Path, Line: violation.Predicate.Line, Column: violation.Predicate.Column, ArtifactPath: violation.ArtifactPath, Related: locations(violation.RelatedSource), Message: fmt.Sprintf("predicate %s failed for target %s", violation.PredicateID, violation.TargetID)}
	}
	return result
}

func locations(positions []compile.SourcePosition) []Location {
	result := make([]Location, len(positions))
	for index, position := range positions {
		result[index] = Location{Path: position.Path, Line: position.Line, Column: position.Column}
	}
	return result
}

type localReader struct{}

func (localReader) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

type localWriter struct {
	root string
}

func (writer localWriter) Write(path string, data []byte) error {
	abs := filepath.Join(writer.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

type outputComparison struct {
	Path     string
	Expected []byte
	Actual   []byte
	Err      error
}

func compareOutput(output outputComparison) (Diagnostic, bool) {
	if output.Err != nil {
		if errors.Is(output.Err, os.ErrNotExist) {
			return Diagnostic{Kind: "output_mismatch", Path: output.Path, Message: "output is missing"}, true
		}
		return Diagnostic{Kind: "output_error", Path: output.Path, Message: output.Err.Error()}, true
	}
	if !bytes.Equal(output.Actual, output.Expected) {
		return Diagnostic{Kind: "output_mismatch", Path: output.Path, Message: "output bytes differ"}, true
	}
	return Diagnostic{}, false
}
