package lockfile

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/lexer"
)

func TestCanonicalBytesUseNormativeOrderEscapingAndValidatedContractData(t *testing.T) {
	registry := config.ContractRegistry{Contracts: []config.Contract{{
		ID: "ban", Kind: config.PredicateForbids, Pattern: "<>&é\u2028\u007f", AppliesTo: []string{"two", "one"},
	}}}
	units := []compile.SourceUnit{{Path: "src/é.md", IR: lexer.IR{Markers: []lexer.Marker{
		{Kind: lexer.ContractOpen, Token: "span"}, {Kind: lexer.AnchorMarker, Token: "gate"},
	}}}}
	project := config.ProjectConfig{SpecVersion: 2, Project: config.Project{VersionFrom: "VERSION"}, Sources: config.Sources{Files: []string{"src/é.md", "VERSION"}}}
	got := CanonicalBytes(project, registry, units)
	want := "{\n" +
		"  \"spec_version\": 2,\n" +
		"  \"semantic_inputs\": [\n" +
		"    \"gunte.toml\",\n" +
		"    \"contracts.toml\",\n" +
		"    \"VERSION\",\n" +
		"    \"src/é.md\"\n" +
		"  ],\n" +
		"  \"contracts\": [\n" +
		"    {\n      \"id\": \"ban\",\n      \"sha256\": \"80b00e0d7a0a7a38458645523619fb1622ddf25b95f5ce5729c26c7465d01fe0\"\n    }\n" +
		"  ],\n" +
		"  \"declarations\": [\n" +
		"    {\n      \"kind\": \"span\",\n      \"id\": \"span\",\n      \"path\": \"src/é.md\"\n    },\n" +
		"    {\n      \"kind\": \"anchor\",\n      \"id\": \"gate\",\n      \"path\": \"src/é.md\"\n    }\n" +
		"  ]\n" +
		"}\n"
	preimage := ContractPreimage(registry.Contracts[0])
	if string(preimage) != "{\"type\":\"text\",\"id\":\"ban\",\"kind\":\"forbids\",\"slice\":null,\"pattern\":\"<>&é\u2028\\u007f\",\"before\":null,\"after\":null,\"applies_to\":[\"two\",\"one\"]}\n" {
		t.Fatalf("preimage = %q", preimage)
	}
	if len(got) == 0 || got[len(got)-1] != '\n' || strings.Contains(string(got), "\\u003c") || strings.Contains(string(got), "\\u00e9") {
		t.Fatalf("canonical escaping = %q", got)
	}
	if string(got) != want {
		t.Fatalf("canonical bytes = %s\nwant = %s", got, want)
	}
}

func TestSemanticInputPathsTrackSelectedContractFileOrder(t *testing.T) {
	project := config.ProjectConfig{ContractFiles: []string{"rules/b.toml", "rules/a.toml"}, Project: config.Project{VersionFrom: "VERSION"}, Sources: config.Sources{Files: []string{"src/a.md"}}}
	want := []string{"gunte.toml", "rules/b.toml", "rules/a.toml", "VERSION", "src/a.md"}
	if got := SemanticInputPaths(project); !reflect.DeepEqual(got, want) {
		t.Fatalf("SemanticInputPaths() = %v, want %v", got, want)
	}
}

func TestCanonicalLockChangesWhenSourcePathInventoryChanges(t *testing.T) {
	project := config.ProjectConfig{SpecVersion: 2, Sources: config.Sources{Files: []string{"src/a.md"}}}
	before := CanonicalBytes(project, config.ContractRegistry{}, nil)
	project.Sources.Files = append(project.Sources.Files, "src/b.md")
	withAddedPath := CanonicalBytes(project, config.ContractRegistry{}, nil)
	if string(before) == string(withAddedPath) {
		t.Fatal("added source path did not change lock bytes")
	}
	project.Sources.Files = project.Sources.Files[:1]
	if afterRemoval := CanonicalBytes(project, config.ContractRegistry{}, nil); string(afterRemoval) != string(before) {
		t.Fatal("removing source path did not restore lock bytes")
	}
}

func TestAtomicWriteSyncsAndClosesBeforeRenameThenConfirmsBytes(t *testing.T) {
	operations := &recordingAtomicOperations{file: &recordingSyncedFile{name: "/project/.gunte.lock.tmp"}, old: []byte("old"), observed: []byte("new")}
	if err := writeAtomic("/project/gunte.lock.json", []byte("new"), operations); err != nil {
		t.Fatal(err)
	}
	want := "read,create,chmod,write,sync,close,rename,read"
	if strings.Join(operations.events, ",") != want {
		t.Fatalf("events = %v, want %s", operations.events, want)
	}
	if operations.createDirectory != "/project" || operations.createPattern != ".gunte.lock.*" {
		t.Fatalf("temp location = %q %q", operations.createDirectory, operations.createPattern)
	}
}

func TestAtomicWriteKeepsOldLockWhenSyncFailsBeforeRename(t *testing.T) {
	operations := &recordingAtomicOperations{file: &recordingSyncedFile{name: "/project/.gunte.lock.tmp", syncErr: errors.New("sync failed")}, old: []byte("old"), observed: []byte("old")}
	if err := writeAtomic("/project/gunte.lock.json", []byte("new"), operations); err == nil {
		t.Fatal("write succeeded")
	}
	if strings.Contains(strings.Join(operations.events, ","), "rename") || !strings.Contains(strings.Join(operations.events, ","), "remove") {
		t.Fatalf("events = %v", operations.events)
	}
	if string(operations.observed) != "old" {
		t.Fatalf("old lock changed to %q", operations.observed)
	}
}

func TestAtomicWriteCleansTempWithoutRenameOnWriteOrCloseFailure(t *testing.T) {
	tests := []struct {
		name string
		file *recordingSyncedFile
	}{
		{"short write", &recordingSyncedFile{name: "/project/.gunte.lock.tmp", shortWrite: true}},
		{"close", &recordingSyncedFile{name: "/project/.gunte.lock.tmp", closeErr: errors.New("close failed")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operations := &recordingAtomicOperations{file: test.file, old: []byte("old"), observed: []byte("old")}
			if err := writeAtomic("/project/gunte.lock.json", []byte("new"), operations); err == nil {
				t.Fatal("write succeeded")
			}
			events := strings.Join(operations.events, ",")
			if strings.Contains(events, "rename") || !strings.Contains(events, "remove") {
				t.Fatalf("events = %v", operations.events)
			}
		})
	}
}

func TestAtomicWriteObservesUnknownRenameResultWithoutBlindRetry(t *testing.T) {
	operations := &recordingAtomicOperations{file: &recordingSyncedFile{name: "/project/.gunte.lock.tmp"}, renameErr: errors.New("result unknown"), old: []byte("old"), observed: []byte("new")}
	if err := writeAtomic("/project/gunte.lock.json", []byte("new"), operations); err != nil {
		t.Fatal(err)
	}
	if operations.renameCount != 1 {
		t.Fatalf("rename count = %d", operations.renameCount)
	}
	operations = &recordingAtomicOperations{file: &recordingSyncedFile{name: "/project/.gunte.lock.tmp"}, renameErr: errors.New("result unknown"), old: []byte("old"), observed: []byte("old")}
	if err := writeAtomic("/project/gunte.lock.json", []byte("new"), operations); err == nil || !strings.Contains(err.Error(), "old lock") || operations.renameCount != 1 {
		t.Fatalf("old observation = %v, renames = %d", err, operations.renameCount)
	}
	operations = &recordingAtomicOperations{file: &recordingSyncedFile{name: "/project/.gunte.lock.tmp"}, renameErr: errors.New("result unknown"), old: []byte("old"), observed: []byte("other")}
	if err := writeAtomic("/project/gunte.lock.json", []byte("new"), operations); err == nil || operations.renameCount != 1 {
		t.Fatalf("write = %v, renames = %d", err, operations.renameCount)
	}
}

type recordingAtomicOperations struct {
	file            *recordingSyncedFile
	events          []string
	renameErr       error
	renameCount     int
	readCount       int
	old             []byte
	observed        []byte
	createDirectory string
	createPattern   string
}

func (operations *recordingAtomicOperations) CreateTemp(directory, pattern string) (syncedFile, error) {
	operations.events = append(operations.events, "create")
	operations.createDirectory, operations.createPattern = directory, pattern
	operations.file.events = &operations.events
	return operations.file, nil
}
func (operations *recordingAtomicOperations) Rename(_, _ string) error {
	operations.events = append(operations.events, "rename")
	operations.renameCount++
	return operations.renameErr
}
func (operations *recordingAtomicOperations) ReadFile(string) ([]byte, error) {
	operations.events = append(operations.events, "read")
	operations.readCount++
	if operations.readCount == 1 {
		if operations.old == nil {
			return nil, os.ErrNotExist
		}
		return append([]byte(nil), operations.old...), nil
	}
	return append([]byte(nil), operations.observed...), nil
}
func (operations *recordingAtomicOperations) Remove(string) error {
	operations.events = append(operations.events, "remove")
	return nil
}

type recordingSyncedFile struct {
	name       string
	events     *[]string
	syncErr    error
	closeErr   error
	shortWrite bool
}

func (file *recordingSyncedFile) Name() string { return file.name }
func (file *recordingSyncedFile) Chmod(os.FileMode) error {
	*file.events = append(*file.events, "chmod")
	return nil
}
func (file *recordingSyncedFile) Write(body []byte) (int, error) {
	*file.events = append(*file.events, "write")
	if file.shortWrite {
		return len(body) - 1, nil
	}
	return len(body), nil
}
func (file *recordingSyncedFile) Sync() error {
	*file.events = append(*file.events, "sync")
	return file.syncErr
}
func (file *recordingSyncedFile) Close() error {
	*file.events = append(*file.events, "close")
	return file.closeErr
}

func TestSemanticInputsFollowNormativeOrderAndRemoveDuplicates(t *testing.T) {
	project := config.ProjectConfig{Project: config.Project{VersionFrom: "VERSION"}, Sources: config.Sources{Files: []string{"src/a", "VERSION", "src/a"}}}
	got := SemanticInputPaths(project)
	want := []string{"gunte.toml", "contracts.toml", "VERSION", "src/a"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

func TestStructureContractPreimagePreservesTypedValuesAndCanonicalKeyOrder(t *testing.T) {
	value := config.TypedValue{Kind: config.TypedMap, Map: []config.TypedEntry{
		{Key: "é", Value: config.TypedValue{Kind: config.TypedBool, Bool: true}},
		{Key: "a", Value: config.TypedValue{Kind: config.TypedInt, Int: 2}},
	}}
	contract := config.Contract{ID: "shape", Kind: config.PredicateStructure, Subject: config.StructureSourceFrontmatter, Paths: []string{"src/*.md"}, Assertions: []config.StructureAssertion{{Path: "policy", Op: config.AssertEquals, Value: &value}}}
	want := "{\"type\":\"structure\",\"id\":\"shape\",\"subject\":\"source_frontmatter\",\"paths\":[\"src/*.md\"],\"format\":null,\"applies_to\":[],\"assertions\":[{\"path\":\"policy\",\"op\":\"equals\",\"value\":{\"a\":2,\"é\":true},\"count\":null}]}\n"
	if got := string(ContractPreimage(contract)); got != want {
		t.Fatalf("preimage = %s, want %s", got, want)
	}
}

func TestCanonicalLockHashChangesWhenStructureTypedPolicyChanges(t *testing.T) {
	value := config.TypedValue{Kind: config.TypedBool, Bool: true}
	contract := config.Contract{ID: "shape", Kind: config.PredicateStructure, Subject: config.StructureSourceFrontmatter, Paths: []string{"src/*.md"}, Assertions: []config.StructureAssertion{{Path: "policy", Op: config.AssertEquals, Value: &value}}}
	project := config.ProjectConfig{SpecVersion: 2}
	before := CanonicalBytes(project, config.ContractRegistry{Contracts: []config.Contract{contract}}, nil)
	value.Bool = false
	after := CanonicalBytes(project, config.ContractRegistry{Contracts: []config.Contract{contract}}, nil)
	if string(before) == string(after) {
		t.Fatal("typed structure policy change did not change lock hash")
	}
}
