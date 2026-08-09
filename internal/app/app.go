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
	"sort"
	"strings"

	"github.com/akitanabe/gunte/internal/adapter"
	"github.com/akitanabe/gunte/internal/cli"
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/contract"
	"github.com/akitanabe/gunte/internal/inventory"
	"github.com/akitanabe/gunte/internal/lexer"
	"github.com/akitanabe/gunte/internal/lockfile"
	"github.com/akitanabe/gunte/internal/outputpath"
	"github.com/akitanabe/gunte/internal/serialize"
	"github.com/akitanabe/gunte/internal/source"
	"github.com/akitanabe/gunte/internal/structure"
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
	Kind          string
	Path          string
	Line          int
	Column        int
	ArtifactPath  string
	Related       []Location
	ActualCount   *int64
	ExpectedCount *int64
	Message       string
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

type directoryReader interface {
	ReadDir(path string) ([]os.DirEntry, error)
	Lstat(path string) (os.FileInfo, error)
}

type writer interface {
	Write(path string, data []byte) error
}

type lockWriter interface {
	Write(path string, data []byte) error
}

// Runner executes one emit or check command for a fixed project root.
type Runner struct {
	root   string
	reader reader
	dirs   directoryReader
	writer writer
	lock   lockWriter
}

// NewRunner creates a runner backed by the local filesystem.
func NewRunner(root string) Runner {
	return Runner{root: root, reader: localReader{}, dirs: localReader{}, writer: localWriter{root: root}, lock: atomicLockWriter{}}
}

func newRunnerWithWriter(root string, output writer) Runner {
	return Runner{root: root, reader: localReader{}, dirs: localReader{}, writer: output, lock: atomicLockWriter{}}
}

func newRunnerWithReader(root string, input reader, output writer) Runner {
	return Runner{root: root, reader: input, dirs: localReader{}, writer: output, lock: atomicLockWriter{}}
}

func newRunnerWithWriters(root string, output writer, lockOutput lockWriter) Runner {
	return Runner{root: root, reader: localReader{}, dirs: localReader{}, writer: output, lock: lockOutput}
}

// Run keeps the argv-based API for compatibility with existing callers.
func (runner Runner) Run(args []string) Result {
	parsed := cli.Parse(args)
	switch parsed.Kind {
	case cli.ParseUsageError:
		return Result{ExitCode: ExitUsage, Diagnostics: []Diagnostic{{Kind: "usage", Message: parsed.Message}}}
	case cli.ParseHelp:
		return Result{ExitCode: ExitSuccess}
	case cli.ParseExecute:
		return runner.Execute(parsed.Request)
	default:
		return Result{ExitCode: ExitUsage, Diagnostics: []Diagnostic{{Kind: "usage", Message: cli.UsageMessage}}}
	}
}

// Execute runs a parsed request against the fixed project root. The request is
// data produced by cli.Parse; argv is not re-parsed at this boundary.
func (runner Runner) Execute(request cli.ExecuteRequest) Result {
	if !request.Valid() {
		return Result{ExitCode: ExitUsage, Diagnostics: []Diagnostic{{Kind: "usage", Message: cli.UsageMessage}}}
	}
	command := string(request.Command)
	selected := request.Target
	project, diagnostics := runner.loadProject()
	if len(diagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: diagnostics}
	}
	if selected != "" && !containsTarget(project, selected) {
		return Result{ExitCode: ExitFailure, Diagnostics: []Diagnostic{{Kind: "unknown_target", Path: "gunte.toml", Message: "unknown target " + selected}}}
	}
	if command == "lock" && project.SpecVersion != 2 {
		return Result{ExitCode: ExitFailure, Diagnostics: []Diagnostic{{Kind: "config_error", Path: "gunte.toml", Line: 1, Column: 1, Message: "gunte lock requires spec_version = 2"}}}
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
	if registryDiagnostics := compile.ValidateRegistryIntegrity(project.SpecVersion, registry, units); len(registryDiagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: sourceDiagnosticResults(registryDiagnostics)}
	}
	sourceDocuments := make([]structure.SourceDocument, len(units))
	for index, unit := range units {
		sourceDocuments[index] = structure.SourceDocument{Path: unit.Path, Node: unit.Document.FrontmatterNode}
	}
	if failures := structure.EvaluateSources(registry, sourceDocuments); len(failures) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: structureFailureResults(failures)}
	}
	artifacts, unmatchedSources, diagnostics := runner.generate(project, projection, units, selected)
	if len(diagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: diagnostics}
	}
	if failures := structure.EvaluateArtifacts(registry, artifacts, selectedIDs(selected)); len(failures) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: structureFailureResults(failures)}
	}
	contractResult, contractDiagnostics := contract.EvaluateTargets(registry, artifacts, selectedIDs(selected))
	if len(contractDiagnostics) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: contractDiagnosticResults(contractDiagnostics)}
	}
	if len(contractResult.Violations) != 0 {
		return Result{ExitCode: ExitFailure, Diagnostics: violationResults(contractResult.Violations)}
	}
	lockBytes := calculateLock(project, registry, units)
	if command == "lock" {
		if err := runner.lock.Write(filepath.Join(runner.root, lockfile.Path), lockBytes); err != nil {
			return Result{ExitCode: ExitFailure, Diagnostics: []Diagnostic{{Kind: "write_error", Path: lockfile.Path, Message: err.Error()}}}
		}
		return Result{ExitCode: ExitSuccess}
	}
	if command == "check" {
		result := runner.check(artifacts)
		if project.SpecVersion == 2 {
			for _, path := range unmatchedSources {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Kind: "unmatched_source", Path: path, Message: "source does not match a rule in any target"})
			}
			result.Diagnostics = append(result.Diagnostics, runner.checkInventory(project, artifacts, selected)...)
			if len(result.Diagnostics) != 0 {
				result.ExitCode = ExitFailure
			}
			data, err := runner.read(lockfile.Path)
			if _, failed := compareOutput(outputComparison{Path: lockfile.Path, Expected: lockBytes, Actual: data, Err: err}); failed {
				result.ExitCode = ExitFailure
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Kind: "lock_mismatch", Path: lockfile.Path, Message: "full semantic lock is missing or differs"})
			}
		}
		return result
	}
	return runner.emit(artifacts)
}

func structureFailureResults(failures []structure.Failure) []Diagnostic {
	result := make([]Diagnostic, len(failures))
	for i, failure := range failures {
		related := []Location(nil)
		if failure.RelatedPath != "" {
			related = []Location{{Path: failure.RelatedPath, Line: 1, Column: 1}}
		}
		result[i] = Diagnostic{Kind: "structure_violation", Path: failure.Contract.Path, Line: failure.Contract.Line, Column: failure.Contract.Column, ArtifactPath: failure.Path, Related: related, Message: failure.Message}
	}
	return result
}

func (runner Runner) loadProject() (config.ProjectConfig, []Diagnostic) {
	data, err := runner.read("gunte.toml")
	if err != nil {
		return config.ProjectConfig{}, []Diagnostic{{Kind: "load_error", Path: "gunte.toml", Message: err.Error()}}
	}
	project, configDiagnostics := config.ParseProject("gunte.toml", data)
	if len(configDiagnostics) != 0 || project.Project.VersionFrom == "" {
		return project, configDiagnosticResults(configDiagnostics)
	}
	versionBytes, err := runner.read(project.Project.VersionFrom)
	if err != nil {
		return project, []Diagnostic{{Kind: "load_error", Path: project.Project.VersionFrom, Message: err.Error()}}
	}
	version, versionDiagnostic := config.NormalizeVersionFile(project.Project.VersionFrom, versionBytes)
	if versionDiagnostic != nil {
		return project, configDiagnosticResults([]config.Diagnostic{*versionDiagnostic})
	}
	project.Project.Version = version
	return project, nil
}

func (runner Runner) loadRegistry(project config.ProjectConfig) (config.ContractRegistry, []Diagnostic) {
	documents := make([]config.ContractDocument, 0, len(project.ContractFiles))
	var diagnostics []Diagnostic
	for _, path := range project.ContractFiles {
		data, err := runner.read(path)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Kind: "load_error", Path: path, Message: err.Error()})
			continue
		}
		documents = append(documents, config.ContractDocument{Path: path, Bytes: data})
	}
	if len(diagnostics) != 0 {
		return config.ContractRegistry{}, diagnostics
	}
	registry, configDiagnostics := config.ParseContractDocuments(documents, project.TargetIDs(), project.SpecVersion)
	return registry, configDiagnosticResults(configDiagnostics)
}

func calculateLock(project config.ProjectConfig, registry config.ContractRegistry, units []compile.SourceUnit) []byte {
	if project.SpecVersion != 2 {
		return nil
	}
	return lockfile.CanonicalBytes(project, registry, units)
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

func (runner Runner) generate(project config.ProjectConfig, projection compile.Result, units []compile.SourceUnit, selected string) ([]serialize.Artifact, []string, []Diagnostic) {
	var artifacts []serialize.Artifact
	var diagnostics []Diagnostic
	plan, preflightDiagnostics := adapter.Preflight(project, adapterSources(units))
	for _, diagnostic := range preflightDiagnostics {
		if diagnostic.Severity == adapter.SeverityError {
			diagnostics = append(diagnostics, Diagnostic{Kind: diagnostic.Code, Path: diagnostic.Source, Message: diagnostic.Message})
		}
	}
	if len(diagnostics) != 0 {
		return nil, plan.UnmatchedSources, diagnostics
	}
	for targetIndex, target := range project.Targets {
		if selected != "" && target.ID != selected {
			continue
		}
		if targetIndex >= len(projection.Targets) {
			return nil, plan.UnmatchedSources, []Diagnostic{{Kind: "compile_error", Path: target.ID, Message: "missing target projection"}}
		}
		targetSources := make([]adapter.Source, len(units))
		for sourceIndex, unit := range units {
			targetSources[sourceIndex] = adapter.Source{Projection: projection.Targets[targetIndex].Sources[sourceIndex], Frontmatter: unit.Document.FrontmatterData}
		}
		adapted, adapterDiagnostics := adapter.AdaptTarget(project, targetIndex, targetSources, plan)
		for _, diagnostic := range adapterDiagnostics {
			if diagnostic.Severity == adapter.SeverityError {
				diagnostics = append(diagnostics, Diagnostic{Kind: diagnostic.Code, Path: diagnostic.Source, Message: diagnostic.Message})
			}
		}
		if len(diagnostics) != 0 {
			return nil, plan.UnmatchedSources, diagnostics
		}
		for _, logical := range adapted.Artifacts {
			serialized, serializeDiagnostics := serialize.Serialize(logical)
			if len(serializeDiagnostics) != 0 {
				for _, diagnostic := range serializeDiagnostics {
					diagnostics = append(diagnostics, Diagnostic{Kind: diagnostic.Code, Path: diagnostic.Path, Message: diagnostic.Message})
				}
				return nil, plan.UnmatchedSources, diagnostics
			}
			artifacts = append(artifacts, serialized)
		}
	}
	return artifacts, plan.UnmatchedSources, nil
}

func adapterSources(units []compile.SourceUnit) []adapter.Source {
	result := make([]adapter.Source, len(units))
	for index, unit := range units {
		result[index] = adapter.Source{Projection: compile.SourceProjection{Path: unit.Path}, Frontmatter: unit.Document.FrontmatterData}
	}
	return result
}

func (runner Runner) checkInventory(project config.ProjectConfig, artifacts []serialize.Artifact, selected string) []Diagnostic {
	expectedSource := config.SemanticInputPaths(project)[1:]
	result := make([]Diagnostic, 0)
	for _, root := range project.Sources.ManagedRoots {
		scope := inventory.Scope{Root: root, AllowFiles: project.Sources.AllowFiles, AllowDirs: project.Sources.AllowDirs}
		entries, diagnostics := runner.snapshot(scope.Root)
		result = append(result, diagnostics...)
		for _, diagnostic := range inventory.Compare(scope, expectedSource, entries) {
			result = append(result, Diagnostic{Kind: "inventory_mismatch", Path: diagnostic.Path, Message: diagnostic.Message})
		}
	}
	for _, target := range project.Targets {
		if selected != "" && target.ID != selected {
			continue
		}
		expected := make([]string, 0)
		for _, artifact := range artifacts {
			if artifact.TargetID == target.ID {
				expected = append(expected, artifact.Path)
			}
		}
		for _, root := range target.ManagedRoots {
			fullRoot := outputpath.Join(target.OutputRoot, root)
			allowFiles := prefixPaths(target.OutputRoot, target.AllowFiles)
			allowDirs := prefixPaths(target.OutputRoot, target.AllowDirs)
			scope := inventory.Scope{Root: fullRoot, AllowFiles: allowFiles, AllowDirs: allowDirs}
			entries, diagnostics := runner.snapshot(fullRoot)
			result = append(result, diagnostics...)
			for _, diagnostic := range inventory.Compare(scope, expected, entries) {
				result = append(result, Diagnostic{Kind: "inventory_mismatch", Path: diagnostic.Path, Message: diagnostic.Message})
			}
		}
	}
	sort.SliceStable(result, func(first, second int) bool {
		if result[first].Path != result[second].Path {
			return result[first].Path < result[second].Path
		}
		return result[first].Kind < result[second].Kind
	})
	return result
}

func prefixPaths(prefix string, paths []string) []string {
	result := make([]string, len(paths))
	for index, path := range paths {
		result[index] = outputpath.Join(prefix, path)
	}
	return result
}

func (runner Runner) snapshot(root string) ([]inventory.Entry, []Diagnostic) {
	abs := filepath.Join(runner.root, filepath.FromSlash(root))
	info, err := runner.dirs.Lstat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []Diagnostic{{Kind: "inventory_error", Path: root, Message: err.Error()}}
	}
	entries := make([]inventory.Entry, 0)
	diagnostics := make([]Diagnostic, 0)
	var walk func(string, string, os.FileInfo)
	walk = func(relative, absolute string, current os.FileInfo) {
		entries = append(entries, inventory.Entry{Path: relative, Kind: inventoryKind(current.Mode())})
		if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() {
			return
		}
		children, readErr := runner.dirs.ReadDir(absolute)
		if readErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Kind: "inventory_error", Path: relative, Message: readErr.Error()})
			return
		}
		for _, child := range children {
			childRelative := relative + "/" + child.Name()
			childAbsolute := filepath.Join(absolute, child.Name())
			childInfo, statErr := runner.dirs.Lstat(childAbsolute)
			if statErr != nil {
				diagnostics = append(diagnostics, Diagnostic{Kind: "inventory_error", Path: childRelative, Message: statErr.Error()})
				continue
			}
			walk(childRelative, childAbsolute, childInfo)
		}
	}
	walk(root, abs, info)
	return entries, diagnostics
}

func inventoryKind(mode os.FileMode) inventory.Kind {
	if mode&os.ModeSymlink != 0 {
		return inventory.KindSymlink
	}
	if mode.IsDir() {
		return inventory.KindDir
	}
	if mode.IsRegular() {
		return inventory.KindFile
	}
	return inventory.KindOther
}

func (runner Runner) emit(artifacts []serialize.Artifact) Result {
	result := Result{ExitCode: ExitSuccess}
	for _, artifact := range artifacts {
		if err := runner.rejectSymlinkPath(artifact.Path); err != nil {
			return Result{ExitCode: ExitFailure, Diagnostics: []Diagnostic{pathSafetyDiagnostic(artifact.Path, err)}}
		}
	}
	for _, artifact := range artifacts {
		// The all-artifacts preflight cannot close a later replacement window, so
		// retain this boundary check immediately before each write.
		if err := runner.rejectSymlinkPath(artifact.Path); err != nil {
			return Result{ExitCode: ExitFailure, Diagnostics: []Diagnostic{pathSafetyDiagnostic(artifact.Path, err)}}
		}
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
		if err := runner.rejectSymlinkPath(artifact.Path); err != nil {
			result.Diagnostics = append(result.Diagnostics, pathSafetyDiagnostic(artifact.Path, err))
			continue
		}
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

var errPathSymlink = errors.New("artifact path contains symlink")

func (runner Runner) rejectSymlinkPath(relative string) error {
	return rejectSymlinkComponents(runner.root, relative, runner.dirs.Lstat)
}

func rejectSymlinkComponents(root, relative string, lstat func(string) (os.FileInfo, error)) error {
	current := root
	for _, component := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", errPathSymlink, current)
		}
	}
	return nil
}

func pathSafetyDiagnostic(path string, err error) Diagnostic {
	if errors.Is(err, errPathSymlink) {
		return Diagnostic{Kind: "path_symlink", Path: path, Message: errPathSymlink.Error()}
	}
	return Diagnostic{Kind: "path_error", Path: path, Message: err.Error()}
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
		related := make([]Location, len(diagnostic.Related))
		for i, position := range diagnostic.Related {
			related[i] = Location{Path: position.Path, Line: position.Line, Column: position.Column}
		}
		result[index] = Diagnostic{Kind: "config_error", Path: diagnostic.Path, Line: diagnostic.Line, Column: diagnostic.Column, Related: related, Message: diagnostic.Message}
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
		message := fmt.Sprintf("predicate %s failed for target %s", violation.PredicateID, violation.TargetID)
		if violation.ActualCount != nil || violation.ExpectedCount != nil {
			message = fmt.Sprintf("%s: expected count %s, actual count %s", message, countText(violation.ExpectedCount), countText(violation.ActualCount))
		}
		result[index] = Diagnostic{Kind: string(violation.Kind), Path: violation.Predicate.Path, Line: violation.Predicate.Line, Column: violation.Predicate.Column, ArtifactPath: violation.ArtifactPath, Related: locations(violation.RelatedSource), ActualCount: violation.ActualCount, ExpectedCount: violation.ExpectedCount, Message: message}
	}
	return result
}

func countText(value *int64) string {
	if value == nil {
		return "<none>"
	}
	return fmt.Sprintf("%d", *value)
}

func locations(positions []compile.SourcePosition) []Location {
	result := make([]Location, len(positions))
	for index, position := range positions {
		result[index] = Location{Path: position.Path, Line: position.Line, Column: position.Column}
	}
	return result
}

type localReader struct{}

type atomicLockWriter struct{}

func (atomicLockWriter) Write(path string, data []byte) error {
	return lockfile.WriteAtomic(path, data)
}

func (localReader) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localReader) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (localReader) Lstat(path string) (os.FileInfo, error)     { return os.Lstat(path) }

type localWriter struct {
	root string
}

func (writer localWriter) Write(path string, data []byte) error {
	if err := rejectSymlinkComponents(writer.root, path, os.Lstat); err != nil {
		return err
	}
	abs := filepath.Join(writer.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(writer.root, path, os.Lstat); err != nil {
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
