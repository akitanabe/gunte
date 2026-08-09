package app

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/cli"
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/contract"
)

func TestRunRejectsUsageAndUnknownTarget(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	runner := NewRunner(root)
	for _, args := range [][]string{{}, {"unknown"}, {"emit", "extra"}, {"emit", "--target"}} {
		result := runner.Run(args)
		if result.ExitCode != ExitUsage || len(result.Diagnostics) != 1 || result.Diagnostics[0].Message != "usage: gunte emit|check [--target ID] | gunte lock" {
			t.Errorf("args %v exit = %d, want %d", args, result.ExitCode, ExitUsage)
		}
	}
	result := runner.Run([]string{"emit", "--target", "missing"})
	if result.ExitCode != ExitFailure || !hasDiagnostic(result, "unknown_target") {
		t.Fatalf("unknown target result = %#v", result)
	}
	result = runner.Run([]string{"emit", "--target", "--help"})
	if result.ExitCode != ExitFailure || !hasDiagnostic(result, "unknown_target") {
		t.Fatalf("help-shaped target result = %#v", result)
	}
}

func TestRunHelpDoesNotReadProjectOrWriteArtifacts(t *testing.T) {
	reader := &countingReader{}
	writer := &recordingWriter{}
	runner := newRunnerWithReader(t.TempDir(), reader, writer)
	for _, args := range [][]string{{"--help"}, {"help", "emit"}, {"check", "--help"}} {
		result := runner.Run(args)
		if result.ExitCode != ExitSuccess || reader.count != 0 || writer.count != 0 {
			t.Fatalf("Run(%v) = %#v reads=%d writes=%d", args, result, reader.count, writer.count)
		}
	}
}

func TestExecuteRejectsInvalidRequestBeforeProjectActions(t *testing.T) {
	for _, request := range []cli.ExecuteRequest{
		{Command: cli.Command("unknown")},
		{Command: cli.CommandLock, Target: "one"},
	} {
		reader := &countingReader{}
		writer := &recordingWriter{}
		result := newRunnerWithReader(t.TempDir(), reader, writer).Execute(request)
		if result.ExitCode != ExitUsage || len(result.Diagnostics) != 1 || result.Diagnostics[0].Message != cli.UsageMessage {
			t.Errorf("Execute(%#v) = %#v, want usage", request, result)
		}
		if reader.count != 0 || writer.count != 0 {
			t.Errorf("Execute(%#v) performed project actions: reads=%d writes=%d", request, reader.count, writer.count)
		}
	}
}

func TestRunEmitIsDeterministicAndCheckDoesNotWrite(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	runner := NewRunner(root)
	first := runner.Run([]string{"emit"})
	if first.ExitCode != ExitSuccess {
		t.Fatalf("first emit = %#v", first)
	}
	path := filepath.Join(root, "out", "a.md")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second := runner.Run([]string{"emit"})
	if second.ExitCode != ExitSuccess {
		t.Fatalf("second emit = %#v", second)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("second emit changed bytes: %q != %q", got, want)
	}
	checkWriter := &recordingWriter{}
	check := newRunnerWithWriter(root, checkWriter).Run([]string{"check"})
	if check.ExitCode != ExitSuccess || checkWriter.count != 0 {
		t.Fatalf("check = %#v", check)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mismatchWriter := &recordingWriter{}
	mismatch := newRunnerWithWriter(root, mismatchWriter).Run([]string{"check"})
	if mismatch.ExitCode != ExitFailure || mismatchWriter.count != 0 || !hasDiagnostic(mismatch, "output_mismatch") {
		t.Fatalf("mismatch = %#v", mismatch)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	missingWriter := &recordingWriter{}
	missing := newRunnerWithWriter(root, missingWriter).Run([]string{"check"})
	if missing.ExitCode != ExitFailure || missingWriter.count != 0 || !hasDiagnostic(missing, "output_mismatch") {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestEmitUsesProjectRelativePathsAndProducesRootIndependentBytes(t *testing.T) {
	firstRoot := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	secondRoot := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	if result := NewRunner(firstRoot).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("first root emit = %#v", result)
	}
	if result := NewRunner(secondRoot).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("second root emit = %#v", result)
	}
	for _, relative := range []string{"out/one/a.md", "out/two/a.md"} {
		first, err := os.ReadFile(filepath.Join(firstRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(secondRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%s differs across roots: %q != %q", relative, first, second)
		}
	}
}

func TestRunTargetSelectionWritesOnlySelectedTarget(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	result := NewRunner(root).Run([]string{"emit", "--target", "two"})
	if result.ExitCode != ExitSuccess {
		t.Fatalf("selected emit = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "two", "a.md")); err != nil {
		t.Fatalf("selected output missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "one", "a.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unselected output exists: %v", err)
	}
}

func TestRunValidatesAllTargetsBeforeSelectedGeneration(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two", extra: "unknown = true\n"}}})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit", "--target", "one"})
	if result.ExitCode != ExitFailure || recorder.count != 0 {
		t.Fatalf("invalid unselected target = %#v writes=%d", result, recorder.count)
	}
}

func TestRunDoesNotWriteWhenInputLoadingFailsMidway(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	projectPath := filepath.Join(root, "gunte.toml")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	project = []byte(strings.Replace(string(project), "files = [\"src/a.md\"]", "files = [\"src/a.md\", \"src/missing.md\"]", 1))
	if err := os.WriteFile(projectPath, project, 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "load_error") {
		t.Fatalf("midway source failure = %#v writes=%d", result, recorder.count)
	}
}

func TestRunContinuesOnAdapterWarningWithoutWritingUnmatchedSources(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out", match: "other/*.md"}}})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitSuccess || recorder.count != 0 {
		t.Fatalf("adapter warning = %#v writes=%d", result, recorder.count)
	}
}

func TestRunDoesNotWriteWhenInputGenerationOrContractFails(t *testing.T) {
	for _, test := range []struct {
		name    string
		fixture projectFixture
	}{
		{name: "load", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, missingSource: true}},
		{name: "parse", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, sourceBody: "+++\ninvalid = [\n+++\nbody\n"}},
		{name: "lex", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, sourceBody: "<!-- @contract span -->\nbody\n"}},
		{name: "adapter", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out", extra: "[[targets.one.rules]]\nmatch = \"src/*.md\"\npath = \"duplicate.md\"\nprofile = \"markdown-v1\"\n"}}}},
		{name: "serialize", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out", profile: "json-v1"}}}},
		{name: "contract", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, contracts: "[contracts.bad]\nkind = \"requires\"\nslice = \"missing\"\npattern = \"x\"\napplies_to = [\"one\"]\n"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := writeProject(t, test.fixture)
			recorder := &recordingWriter{}
			result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
			if result.ExitCode != ExitFailure || recorder.count != 0 {
				t.Fatalf("failure result = %#v writes=%d", result, recorder.count)
			}
		})
	}
}

func TestRunDoesNotWriteAfterActualContractViolation(t *testing.T) {
	root := writeProject(t, projectFixture{
		targets:    []targetFixture{{id: "one", root: "out"}},
		sourceBody: "<!-- @contract span -->\nabsent\n<!-- @/contract -->\n",
		contracts:  "[contracts.need]\nkind = \"requires\"\nslice = \"span\"\npattern = \"wanted\"\napplies_to = [\"one\"]\n",
	})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "requires_violation") {
		t.Fatalf("contract violation = %#v writes=%d", result, recorder.count)
	}
}

func TestRunCheckTargetSelectionAndStaleOutputs(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "one", "a.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"check", "--target", "two"}); result.ExitCode != ExitSuccess {
		t.Fatalf("unselected mismatch affected check = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "one", "a.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "out", "stale.md")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"check"}); result.ExitCode != ExitSuccess {
		t.Fatalf("stale output affected check = %#v", result)
	}
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("stale output affected emit = %#v", result)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("stale output was removed: %v", err)
	}
}

func TestCheckMismatchKeepsExistingBytesAndDoesNotCreateMissingOutput(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	path := filepath.Join(root, "out", "a.md")
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"check"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != "output_mismatch" || result.Diagnostics[0].Path != "out/a.md" {
		t.Fatalf("mismatch exact = %#v writes=%d", result, recorder.count)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed\n" {
		t.Fatalf("mismatch modified existing bytes: %q", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	result = newRunnerWithWriter(root, recorder).Run([]string{"check"})
	if result.ExitCode != ExitFailure || len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != "output_mismatch" || result.Diagnostics[0].Path != "out/a.md" {
		t.Fatalf("missing exact = %#v", result)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("check created missing output: %v", err)
	}
}

func TestSelectedTargetIgnoresFailureOnlyInOtherTarget(t *testing.T) {
	root := writeProject(t, projectFixture{
		targets:    []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}},
		sourceBody: "<!-- @contract span -->\nabsent\n<!-- @/contract -->\n",
		contracts:  "[contracts.only-two]\nkind = \"requires\"\nslice = \"span\"\npattern = \"wanted\"\napplies_to = [\"two\"]\n",
	})
	selected := NewRunner(root).Run([]string{"emit", "--target", "one"})
	if selected.ExitCode != ExitSuccess {
		t.Fatalf("selected target with other failure = %#v", selected)
	}
	if result := NewRunner(root).Run([]string{"check", "--target", "one"}); result.ExitCode != ExitSuccess {
		t.Fatalf("selected check with other failure = %#v", result)
	}
}

func TestRunConfigAndRegistryLoadOrParseFailureNeverWrites(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if err := os.Remove(filepath.Join(root, "gunte.toml")); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "load_error") {
		t.Fatalf("project load failure = %#v writes=%d", result, recorder.count)
	}
	root = writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, contracts: "[contracts.bad\n"})
	recorder = &recordingWriter{}
	result = newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "config_error") {
		t.Fatalf("registry parse failure = %#v writes=%d", result, recorder.count)
	}
}

func TestCommandBinaryRunsInProjectRootWithExpectedExitCodesAndLocations(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
	bin := filepath.Join(t.TempDir(), "gunte")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/gunte")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	run := func(args ...string) (int, string) {
		command := exec.Command(bin, args...)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err == nil {
			return 0, string(output)
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode(), string(output)
		}
		t.Fatalf("run CLI: %v", err)
		return -1, string(output)
	}
	if code, output := run("emit"); code != ExitSuccess || output != "" {
		t.Fatalf("CLI emit = code %d output %q", code, output)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "a.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, output := run("check"); code != ExitFailure || !strings.Contains(output, "output_mismatch: out/a.md") {
		t.Fatalf("CLI check = code %d output %q", code, output)
	}
	if code, output := run("unknown"); code != ExitUsage || !strings.Contains(output, "usage: gunte") {
		t.Fatalf("CLI usage = code %d output %q", code, output)
	}
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte("[contracts.need]\nkind = \"requires\"\nslice = \"span\"\npattern = \"wanted\"\napplies_to = [\"one\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("<!-- @contract span -->\nabsent\n<!-- @/contract -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, output := run("emit"); code != ExitFailure || !strings.Contains(output, "requires_violation: contracts.toml:1:1") || !strings.Contains(output, "related src/a.md:") {
		t.Fatalf("CLI contract location = code %d output %q", code, output)
	}
}

func TestCheckReportsNonNotExistReadErrorWithoutWriting(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	fs := &failingReadFileSystem{base: localReader{}, suffix: filepath.Join("out", "a.md")}
	recorder := &recordingWriter{}
	runner := newRunnerWithReader(root, fs, recorder)
	result := runner.Run([]string{"check"})
	want := Diagnostic{Kind: "output_error", Path: "out/a.md", Message: "artifact read failed"}
	if result.ExitCode != ExitFailure || recorder.count != 0 || len(result.Diagnostics) != 1 || !reflect.DeepEqual(result.Diagnostics[0], want) {
		t.Fatalf("output read error = %#v writes=%d, want %#v", result, recorder.count, want)
	}
}

func TestCompareOutputCalculatesMatchMismatchMissingAndReadError(t *testing.T) {
	tests := []struct {
		name   string
		input  outputComparison
		failed bool
		want   Diagnostic
	}{
		{name: "match", input: outputComparison{Path: "out/a.md", Expected: []byte("same"), Actual: []byte("same")}},
		{name: "mismatch", input: outputComparison{Path: "out/a.md", Expected: []byte("want"), Actual: []byte("got")}, failed: true, want: Diagnostic{Kind: "output_mismatch", Path: "out/a.md", Message: "output bytes differ"}},
		{name: "missing", input: outputComparison{Path: "out/a.md", Err: os.ErrNotExist}, failed: true, want: Diagnostic{Kind: "output_mismatch", Path: "out/a.md", Message: "output is missing"}},
		{name: "read error", input: outputComparison{Path: "out/a.md", Err: errors.New("read failed")}, failed: true, want: Diagnostic{Kind: "output_error", Path: "out/a.md", Message: "read failed"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, failed := compareOutput(test.input)
			if failed != test.failed || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("compareOutput() = %#v, %v; want %#v, %v", got, failed, test.want, test.failed)
			}
		})
	}
}

func TestRunReturnsWriteFailure(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	recorder := &recordingWriter{failAt: 2, err: errors.New("disk full")}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 2 || !hasDiagnostic(result, "write_error") {
		t.Fatalf("write failure = %#v writes=%d", result, recorder.count)
	}
}

func TestViolationMappingRetainsPredicateArtifactAndRelatedLocations(t *testing.T) {
	predicate := config.ContractPosition{Path: "contracts.toml", Line: 4, Column: 1}
	before := compile.SourcePosition{Path: "source.md", Offset: 10, Line: 2, Column: 3}
	after := compile.SourcePosition{Path: "source.md", Offset: 30, Line: 5, Column: 2}
	violations := []contract.Violation{
		{Kind: contract.RequiresViolation, PredicateID: "need", TargetID: "one", ArtifactPath: "out/need.md", Predicate: predicate, RelatedSource: []compile.SourcePosition{before}},
		{Kind: contract.ForbidsViolation, PredicateID: "ban", TargetID: "one", ArtifactPath: "out/ban.md", Predicate: predicate, RelatedSource: []compile.SourcePosition{before}},
		{Kind: contract.OrderViolation, PredicateID: "order", TargetID: "one", ArtifactPath: "out/order.md", Predicate: predicate, RelatedSource: []compile.SourcePosition{before, after}},
	}
	result := violationResults(violations)
	want := []Diagnostic{
		{Kind: "requires_violation", Path: "contracts.toml", Line: 4, Column: 1, ArtifactPath: "out/need.md", Related: []Location{{Path: "source.md", Line: 2, Column: 3}}, Message: "predicate need failed for target one"},
		{Kind: "forbids_violation", Path: "contracts.toml", Line: 4, Column: 1, ArtifactPath: "out/ban.md", Related: []Location{{Path: "source.md", Line: 2, Column: 3}}, Message: "predicate ban failed for target one"},
		{Kind: "order_violation", Path: "contracts.toml", Line: 4, Column: 1, ArtifactPath: "out/order.md", Related: []Location{{Path: "source.md", Line: 2, Column: 3}, {Path: "source.md", Line: 5, Column: 2}}, Message: "predicate order failed for target one"},
	}
	for index := range want {
		if !reflect.DeepEqual(result[index], want[index]) {
			t.Fatalf("violation %d = %#v, want %#v", index, result[index], want[index])
		}
	}
}

func TestRunDoesNotWriteWhenArtifactGenerationFails(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two", profile: "json-v1"}}})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "invalid_json") {
		t.Fatalf("late serialization failure = %#v writes=%d", result, recorder.count)
	}
}

func TestRunCheckReportsAllOutputMismatches(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "one", "a.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "out", "two", "a.md")); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"check"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || len(result.Diagnostics) != 2 {
		t.Fatalf("all mismatch diagnostics = %#v", result)
	}
	if !hasDiagnostic(result, "output_mismatch") {
		t.Fatalf("mismatch kind missing = %#v", result)
	}
}

func TestSpecVersionTwoLockBootstrapsAndCheckRequiresFullCanonicalLock(t *testing.T) {
	root := writeVersionTwoProject(t)
	runner := NewRunner(root)
	if result := runner.Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("emit = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "gunte.lock.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("emit created lock: %v", err)
	}
	if result := runner.Run([]string{"check"}); result.ExitCode != ExitFailure || !hasDiagnostic(result, "lock_mismatch") {
		t.Fatalf("check without lock = %#v", result)
	}
	if result := runner.Run([]string{"lock"}); result.ExitCode != ExitSuccess {
		t.Fatalf("lock = %#v", result)
	}
	lockBefore, err := os.ReadFile(filepath.Join(root, "gunte.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if result := runner.Run([]string{"check", "--target", "one"}); result.ExitCode != ExitSuccess {
		t.Fatalf("selected check = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte("[contracts.ban]\nkind = \"forbids\"\npattern = \"never-present\"\napplies_to = [\"one\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := runner.Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("emit after semantic change = %#v", result)
	}
	lockAfter, err := os.ReadFile(filepath.Join(root, "gunte.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lockBefore, lockAfter) {
		t.Fatalf("emit changed lock")
	}
	if result := runner.Run([]string{"check", "--target", "one"}); result.ExitCode != ExitFailure || !hasDiagnostic(result, "lock_mismatch") {
		t.Fatalf("selected check with stale full lock = %#v", result)
	}
}

func TestSpecVersionTwoLockDoesNotChangeWhenOnlySourceContentChanges(t *testing.T) {
	root := writeVersionTwoProject(t)
	runner := NewRunner(root)
	if result := runner.Run([]string{"lock"}); result.ExitCode != ExitSuccess {
		t.Fatalf("initial lock = %#v", result)
	}
	before, err := os.ReadFile(filepath.Join(root, "gunte.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("changed source body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := runner.Run([]string{"lock"}); result.ExitCode != ExitSuccess {
		t.Fatalf("lock after source edit = %#v", result)
	}
	after, err := os.ReadFile(filepath.Join(root, "gunte.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("source content changed lock bytes")
	}
}

func TestLockCommandIsVersionTwoOnlyAndRejectsTargetOption(t *testing.T) {
	v1Root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if result := NewRunner(v1Root).Run([]string{"lock"}); result.ExitCode != ExitFailure || !hasDiagnostic(result, "config_error") {
		t.Fatalf("v1 lock = %#v", result)
	}
	v2Root := writeVersionTwoProject(t)
	if result := NewRunner(v2Root).Run([]string{"lock", "--target", "one"}); result.ExitCode != ExitUsage || result.Diagnostics[0].Message != "usage: gunte emit|check [--target ID] | gunte lock" {
		t.Fatalf("targeted lock = %#v", result)
	}
}

func TestVersionFromSuppliesProjectVersionAndParticipatesInCollision(t *testing.T) {
	root := writeVersionTwoProject(t)
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("\xef\xbb\xbf 2.0 \r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "gunte.toml")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	project = []byte(strings.Replace(string(project), `profile = "markdown-v1"`, "profile = \"plain-text-v1\"\nvalue_from = \"project:version\"", 1))
	if err := os.WriteFile(projectPath, project, 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("emit = %#v", result)
	}
	got, err := os.ReadFile(filepath.Join(root, "out", "a.md"))
	if err != nil || string(got) != " 2.0 \n" {
		t.Fatalf("artifact = %q, %v", got, err)
	}

	collisionRoot := writeVersionTwoProject(t)
	projectPath = filepath.Join(collisionRoot, "gunte.toml")
	project, err = os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	project = []byte(strings.Replace(string(project), `version_from = "VERSION"`, `version_from = "out/a.md"`, 1))
	if err := os.WriteFile(projectPath, project, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(collisionRoot, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collisionRoot, "out", "a.md"), []byte("2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(collisionRoot).Run([]string{"emit"}); result.ExitCode != ExitFailure || !hasDiagnostic(result, "path_collision") {
		t.Fatalf("version_from collision = %#v", result)
	}
}

func TestV2CheckComparesSourceAndSelectedTargetManagedInventoryReadOnly(t *testing.T) {
	root := writeInventoryVersionTwoProject(t)
	runner := NewRunner(root)
	if result := runner.Run([]string{"lock"}); result.ExitCode != ExitSuccess {
		t.Fatalf("lock = %#v", result)
	}
	if result := runner.Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "keep"), []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "out", "generated", "allowed", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "generated", "allowed", "nested", "kept.md"), []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "generated", "stale.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unmanaged.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writer := &recordingWriter{}
	result := newRunnerWithWriter(root, writer).Run([]string{"check"})
	if result.ExitCode != ExitFailure || writer.count != 0 {
		t.Fatalf("inventory check = %#v writes=%d", result, writer.count)
	}
	if !hasDiagnosticPath(result, "inventory_mismatch", "src/empty") || !hasDiagnosticPath(result, "inventory_mismatch", "out/generated/stale.md") {
		t.Fatalf("inventory diagnostics = %#v", result.Diagnostics)
	}
	if hasDiagnosticPath(result, "inventory_mismatch", "unmanaged.txt") {
		t.Fatalf("scope-external path was reported: %#v", result.Diagnostics)
	}
}

func TestV2CheckTargetSelectionStillChecksFullSourceCoverageButOnlySelectedOutputScope(t *testing.T) {
	root := writeInventoryVersionTwoProject(t)
	projectPath := filepath.Join(root, "gunte.toml")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(project), `files = ["src/a.md"]`, `files = ["src/a.md", "src/unmatched.md"]`, 1)
	text = strings.Replace(text, `match = "src/*.md"`, `match = "src/a.md"`, 1)
	text += `[targets.two]
output_root = "other"
managed_roots = ["generated"]
[[targets.two.rules]]
match = "src/a.md"
path = "generated/a.md"
profile = "markdown-v1"
`
	if err := os.WriteFile(projectPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "unmatched.md"), []byte("unmatched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"lock"}); result.ExitCode != ExitSuccess {
		t.Fatalf("lock = %#v", result)
	}
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "stale.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "generated", "stale.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other", "generated", "stale.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := NewRunner(root).Run([]string{"check", "--target", "one"})
	if result.ExitCode != ExitFailure || !hasDiagnosticPath(result, "unmatched_source", "src/unmatched.md") {
		t.Fatalf("selected source coverage = %#v", result)
	}
	if !hasDiagnosticPath(result, "inventory_mismatch", "src/stale.md") || !hasDiagnosticPath(result, "inventory_mismatch", "out/generated/stale.md") {
		t.Fatalf("full-source or selected-output inventory was not reported = %#v", result.Diagnostics)
	}
	if hasDiagnosticPath(result, "inventory_mismatch", "other/generated/stale.md") {
		t.Fatalf("unselected output inventory was reported = %#v", result.Diagnostics)
	}
}

func TestV2SourceInventoryTreatsManagedVersionFileAsExpectedLeaf(t *testing.T) {
	for _, kind := range []string{"regular", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := writeInventoryVersionTwoProject(t)
			projectPath := filepath.Join(root, "gunte.toml")
			project, err := os.ReadFile(projectPath)
			if err != nil {
				t.Fatal(err)
			}
			project = []byte(strings.Replace(string(project), `version_from = "VERSION"`, `version_from = "src/VERSION"`, 1))
			if err := os.WriteFile(projectPath, project, 0o644); err != nil {
				t.Fatal(err)
			}
			if kind == "regular" {
				if err := os.WriteFile(filepath.Join(root, "src", "VERSION"), []byte("2.0\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Symlink("../VERSION", filepath.Join(root, "src", "VERSION")); err != nil {
					t.Fatal(err)
				}
			}
			if result := NewRunner(root).Run([]string{"lock"}); result.ExitCode != ExitSuccess {
				t.Fatalf("lock = %#v", result)
			}
			if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
				t.Fatalf("emit = %#v", result)
			}
			result := NewRunner(root).Run([]string{"check"})
			if hasDiagnosticPath(result, "inventory_mismatch", "src/VERSION") {
				t.Fatalf("version inventory = %#v", result.Diagnostics)
			}
		})
	}
}

func TestV2CheckDoesNotFollowDirectorySymlinkAndAllowsSymlinkAllowFile(t *testing.T) {
	root := writeInventoryVersionTwoProject(t)
	projectPath := filepath.Join(root, "gunte.toml")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	project = []byte(strings.Replace(string(project), `allow_files = ["src/keep"]`, `allow_files = ["src/keep", "src/link-file"]`, 1))
	if err := os.WriteFile(projectPath, project, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("VERSION", filepath.Join(root, "src", "link-file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("generated", filepath.Join(root, "src", "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"lock"}); result.ExitCode != ExitSuccess {
		t.Fatalf("lock = %#v", result)
	}
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("emit = %#v", result)
	}
	if err := os.MkdirAll(filepath.Join(root, "out", "generated", "allowed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(root, "out", "generated", "allowed", "link-dir")); err != nil {
		t.Fatal(err)
	}
	result := NewRunner(root).Run([]string{"check"})
	if result.ExitCode != ExitFailure || !hasDiagnosticPath(result, "inventory_mismatch", "src/linked-dir") {
		t.Fatalf("symlink inventory = %#v", result.Diagnostics)
	}
	if hasDiagnosticPath(result, "inventory_mismatch", "out/generated/allowed/link-dir") {
		t.Fatalf("allow directory descendant symlink was rejected = %#v", result.Diagnostics)
	}
	if hasDiagnosticPath(result, "inventory_mismatch", "src/link-file") {
		t.Fatalf("allow file symlink was rejected = %#v", result.Diagnostics)
	}
}

func writeInventoryVersionTwoProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"VERSION":        "2.0\n",
		"src/a.md":       "hello\n",
		"contracts.toml": "[contracts]\n",
		"gunte.toml": `spec_version = 2
[project]
id = "p"
version_from = "VERSION"
[sources]
files = ["src/a.md"]
managed_roots = ["src"]
allow_files = ["src/keep"]
[targets.one]
output_root = "out"
managed_roots = ["generated"]
allow_files = ["generated/README"]
allow_dirs = ["generated/allowed"]
[[targets.one.rules]]
match = "src/*.md"
path = "generated/a.md"
profile = "markdown-v1"
`,
	}
	for path, body := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestInvalidValidationDoesNotCreateLock(t *testing.T) {
	for _, mutate := range []func(string){
		func(root string) {
			if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("2.0\n\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		func(root string) {
			if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("<!-- @contract unused -->\nbody\n<!-- @/contract -->\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		root := writeVersionTwoProject(t)
		mutate(root)
		if result := NewRunner(root).Run([]string{"lock"}); result.ExitCode != ExitFailure {
			t.Fatalf("invalid lock = %#v", result)
		}
		if _, err := os.Stat(filepath.Join(root, "gunte.lock.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("lock writer reached: %v", err)
		}
	}
}

func TestExplicitContractFilesAreTheOnlyRegistryInputs(t *testing.T) {
	root := writeVersionTwoProject(t)
	projectPath := filepath.Join(root, "gunte.toml")
	projectBytes, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	projectBytes = bytes.Replace(projectBytes, []byte("[sources]"), []byte("[contracts]\nfiles = [\"rules/a.toml\", \"rules/b.toml\"]\n[sources]"), 1)
	if err := os.WriteFile(projectPath, projectBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"rules/a.toml", "rules/b.toml"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("[contracts]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte("not valid toml ="), 0o644); err != nil {
		t.Fatal(err)
	}
	writer := &recordingWriter{}
	if result := newRunnerWithWriter(root, writer).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("explicit registry = %#v", result)
	}
	if err := os.Remove(filepath.Join(root, "rules", "b.toml")); err != nil {
		t.Fatal(err)
	}
	writer = &recordingWriter{}
	result := newRunnerWithWriter(root, writer).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || !hasDiagnosticPath(result, "load_error", "rules/b.toml") || writer.count != 0 {
		t.Fatalf("missing registry = %#v, writes = %d", result, writer.count)
	}
}

func TestInvalidSelectedContractDoesNotWriteArtifactsOrLock(t *testing.T) {
	root := writeVersionTwoProject(t)
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte("[contracts.bad]\nkind = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts := &recordingWriter{}
	locks := &recordingWriter{}
	result := newRunnerWithWriters(root, artifacts, locks).Run([]string{"lock"})
	if result.ExitCode != ExitFailure || artifacts.count != 0 || locks.count != 0 {
		t.Fatalf("result = %#v, artifact writes = %d, lock writes = %d", result, artifacts.count, locks.count)
	}
}

func TestTopLevelInlineContractViolationUsesPredicatePosition(t *testing.T) {
	contracts := `contracts = { ban = { kind = "forbids", pattern = "hello", applies_to = ["one"] } }
`
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, contracts: contracts})
	result := newRunnerWithWriter(root, &recordingWriter{}).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || len(result.Diagnostics) != 1 {
		t.Fatalf("result = %#v", result)
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Kind != "forbids_violation" || diagnostic.Path != "contracts.toml" || diagnostic.Line != 1 || diagnostic.Column != strings.Index(contracts, "ban")+1 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestSourceInventoryExpectsEverySelectedContractFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "registry"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"registry/a.toml", "registry/b.toml"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte("[contracts]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	project := config.ProjectConfig{ContractFiles: []string{"registry/a.toml", "registry/b.toml"}, Sources: config.Sources{ManagedRoots: []string{"registry"}}}
	if diagnostics := NewRunner(root).checkInventory(project, nil, ""); len(diagnostics) != 0 {
		t.Fatalf("inventory diagnostics = %#v", diagnostics)
	}
	project.ContractFiles = []string{"registry/a.toml"}
	diagnostics := NewRunner(root).checkInventory(project, nil, "")
	if len(diagnostics) != 1 || diagnostics[0].Kind != "inventory_mismatch" || diagnostics[0].Path != "registry/b.toml" {
		t.Fatalf("unselected contract diagnostics = %#v", diagnostics)
	}
}

func TestValidationFailuresPreserveExistingLockBytes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string)
	}{
		{name: "structure", mutate: func(root string) {
			contracts := `[contracts.shape]
kind = "structure"
subject = "source_frontmatter"
paths = ["src/*.md"]
[[contracts.shape.assertions]]
path = "missing"
op = "exists"
`
			if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "text contract", mutate: func(root string) {
			contracts := `[contracts.required]
kind = "requires"
pattern = "missing"
applies_to = ["one"]
`
			if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "generation", mutate: func(root string) {
			projectPath := filepath.Join(root, "gunte.toml")
			project, err := os.ReadFile(projectPath)
			if err != nil {
				t.Fatal(err)
			}
			project = []byte(strings.Replace(string(project), `profile = "markdown-v1"`, `profile = "json-v1"`, 1))
			if err := os.WriteFile(projectPath, project, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeVersionTwoProject(t)
			lockPath := filepath.Join(root, "gunte.lock.json")
			before := []byte("existing lock bytes\n")
			if err := os.WriteFile(lockPath, before, 0o644); err != nil {
				t.Fatal(err)
			}
			test.mutate(root)
			if result := NewRunner(root).Run([]string{"lock"}); result.ExitCode != ExitFailure {
				t.Fatalf("lock = %#v", result)
			}
			after, err := os.ReadFile(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("lock changed: %q", after)
			}
		})
	}
}

func TestSelectedEmitStillEvaluatesSourceStructureAcrossFullProject(t *testing.T) {
	root := writeVersionTwoProject(t)
	projectPath := filepath.Join(root, "gunte.toml")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	project = append(project, []byte("[targets.two]\noutput_root = \"out/two\"\n[[targets.two.rules]]\nmatch = \"src/*.md\"\npath = \"a.md\"\nprofile = \"markdown-v1\"\n")...)
	if err := os.WriteFile(projectPath, project, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("+++\npolicy = false\n+++\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contracts := `[contracts.policy]
kind = "structure"
subject = "source_frontmatter"
paths = ["src/*.md"]
[[contracts.policy.assertions]]
path = "policy"
op = "equals"
value = true
`
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
		t.Fatal(err)
	}
	result := newRunnerWithWriter(root, &recordingWriter{}).Run([]string{"emit", "--target", "one"})
	if result.ExitCode != ExitFailure || !hasDiagnostic(result, "structure_violation") || result.Diagnostics[0].Path != "contracts.toml" || len(result.Diagnostics[0].Related) != 1 || result.Diagnostics[0].Related[0].Path != "src/a.md" {
		t.Fatalf("result = %#v", result)
	}
}

func TestArtifactStructureUsesProfileProvenanceAndTypedYAMLFrontmatter(t *testing.T) {
	root := writeVersionTwoProject(t)
	projectPath := filepath.Join(root, "gunte.toml")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	project = []byte(strings.Replace(string(project), `profile = "markdown-v1"`, `profile = "markdown+yaml-frontmatter-v1"`, 1))
	if err := os.WriteFile(projectPath, project, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("---\nenabled: true\n---\nbody\n---\nignored: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contracts := `[contracts.shape]
kind = "structure"
subject = "artifact"
paths = ["out/a.md"]
format = "yaml"
applies_to = ["one"]
[[contracts.shape.assertions]]
path = "enabled"
op = "equals"
value = true
`
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("---\nenabled: false\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := newRunnerWithWriter(root, &recordingWriter{}).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || !hasDiagnostic(result, "structure_violation") || result.Diagnostics[0].Path != "contracts.toml" || result.Diagnostics[0].Line != 1 || result.Diagnostics[0].ArtifactPath != "out/a.md" || len(result.Diagnostics[0].Related) != 1 || result.Diagnostics[0].Related[0].Path != "src/a.md" {
		t.Fatalf("result = %#v", result)
	}
}

func TestArtifactStructureValidatesWholeMarkdownProfileAsYAML(t *testing.T) {
	root := writeVersionTwoProject(t)
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contracts := `[contracts.shape]
kind = "structure"
subject = "artifact"
paths = ["out/a.md"]
format = "yaml"
applies_to = ["one"]
[[contracts.shape.assertions]]
path = ""
op = "equals"
value = true
`
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("initial emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := newRunnerWithWriter(root, &recordingWriter{}).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != "structure_violation" || result.Diagnostics[0].Path != "contracts.toml" || result.Diagnostics[0].ArtifactPath != "out/a.md" || len(result.Diagnostics[0].Related) != 1 || result.Diagnostics[0].Related[0].Path != "src/a.md" {
		t.Fatalf("mutated emit = %#v", result)
	}
}

func TestArtifactStructureRawYAMLSourceMutationsFailBeforeWriterWithOwnership(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		assertion  string
		wantReason string
	}{
		{
			name: "top-level extra key",
			body: "policy:\n  enabled: true\nextra: false\n",
			assertion: `[[contracts.shape.assertions]]
path = ""
op = "exact_keys"
value = ["policy"]`,
			wantReason: "exact_keys",
		},
		{
			name: "nested extra policy key",
			body: "policy:\n  enabled: true\n  extra: false\n",
			assertion: `[[contracts.shape.assertions]]
path = "policy"
op = "exact_keys"
value = ["enabled"]`,
			wantReason: "exact_keys",
		},
		{
			name: "boolean wrong value",
			body: "policy:\n  enabled: false\n",
			assertion: `[[contracts.shape.assertions]]
path = "policy.enabled"
op = "equals"
value = true`,
			wantReason: "equals",
		},
		{
			name: "boolean to string",
			body: "policy:\n  enabled: \"true\"\n",
			assertion: `[[contracts.shape.assertions]]
path = "policy.enabled"
op = "equals"
value = true`,
			wantReason: "equals",
		},
		{
			name: "duplicate key",
			body: "policy:\n  enabled: true\n  enabled: false\n",
			assertion: `[[contracts.shape.assertions]]
path = "policy.enabled"
op = "exists"`,
			wantReason: "duplicate YAML mapping key at policy.enabled",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeVersionTwoProject(t)
			if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte(test.body), 0o644); err != nil {
				t.Fatal(err)
			}
			contracts := `[contracts.shape]
kind = "structure"
subject = "artifact"
paths = ["out/a.md"]
format = "yaml"
applies_to = ["one"]
` + test.assertion + "\n"
			if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
				t.Fatal(err)
			}
			writer := &recordingWriter{}
			result := newRunnerWithWriter(root, writer).Run([]string{"emit"})
			if result.ExitCode != ExitFailure || len(result.Diagnostics) != 1 {
				t.Fatalf("result = %#v", result)
			}
			diagnostic := result.Diagnostics[0]
			if diagnostic.Kind != "structure_violation" || diagnostic.Path != "contracts.toml" || diagnostic.Line != 1 || diagnostic.Column != 1 || diagnostic.ArtifactPath != "out/a.md" || len(diagnostic.Related) != 1 || diagnostic.Related[0].Path != "src/a.md" || !strings.Contains(diagnostic.Message, test.wantReason) || writer.count != 0 {
				t.Fatalf("diagnostic = %#v, writes = %d", diagnostic, writer.count)
			}
		})
	}
}

func writeVersionTwoProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"VERSION":        "2.0\n",
		"src/a.md":       "hello\n",
		"contracts.toml": "[contracts]\n",
		"gunte.toml": `spec_version = 2
[project]
id = "p"
version_from = "VERSION"
[sources]
files = ["src/a.md"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*.md"
path = "a.md"
profile = "markdown-v1"
`,
	}
	for path, body := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

type projectFixture struct {
	targets       []targetFixture
	sourceBody    string
	contracts     string
	missingSource bool
}

type targetFixture struct {
	id      string
	root    string
	extra   string
	match   string
	profile string
}

func writeProject(t *testing.T, fixture projectFixture) string {
	t.Helper()
	root := t.TempDir()
	if fixture.sourceBody == "" {
		fixture.sourceBody = "hello\n"
	}
	if !fixture.missingSource {
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte(fixture.sourceBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var targets strings.Builder
	for _, target := range fixture.targets {
		match := target.match
		if match == "" {
			match = "src/*.md"
		}
		profile := target.profile
		if profile == "" {
			profile = "markdown-v1"
		}
		targets.WriteString("[targets." + target.id + "]\noutput_root = \"" + target.root + "\"\n" + target.extra)
		targets.WriteString("[[targets." + target.id + ".rules]]\nmatch = \"" + match + "\"\npath = \"a.md\"\nprofile = \"" + profile + "\"\n")
	}
	project := "spec_version = 1\n[project]\nid = \"p\"\nversion = \"v\"\n[sources]\nfiles = [\"src/a.md\"]\n" + targets.String()
	if err := os.WriteFile(filepath.Join(root, "gunte.toml"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	contracts := fixture.contracts
	if contracts == "" {
		contracts = "[contracts]\n"
	}
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type recordingWriter struct {
	count  int
	failAt int
	err    error
}

type countingReader struct {
	count int
}

func (reader *countingReader) ReadFile(string) ([]byte, error) {
	reader.count++
	return nil, errors.New("project read must not be called")
}

type failingReadFileSystem struct {
	base   reader
	suffix string
}

func (fs *failingReadFileSystem) ReadFile(path string) ([]byte, error) {
	if strings.HasSuffix(path, fs.suffix) {
		return nil, errors.New("artifact read failed")
	}
	return fs.base.ReadFile(path)
}

func (writer *recordingWriter) Write(path string, body []byte) error {
	writer.count++
	if writer.failAt != 0 && writer.count != writer.failAt {
		return nil
	}
	return writer.err
}

func hasDiagnostic(result Result, kind string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Kind == kind {
			return true
		}
	}
	return false
}

func hasDiagnosticPath(result Result, kind, path string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Kind == kind && diagnostic.Path == path {
			return true
		}
	}
	return false
}
