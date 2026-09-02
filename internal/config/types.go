package config

import "github.com/akitanabe/gunte/internal/typeddata"

// Diagnostic describes a correctable configuration problem.
type Diagnostic struct {
	Path    string
	Line    int
	Column  int
	Related []ContractPosition
	Message string
}

// ProjectConfig is the validated project configuration in declaration order.
type ProjectConfig struct {
	SpecVersion   int
	Project       Project
	ContractFiles []string
	Sources       Sources
	Terms         []Term
	BodyValues    []BodyValue
	Targets       []Target
}

// ContractDocument is one selected registry input already read by the app boundary.
type ContractDocument struct {
	Path  string
	Bytes []byte
}

// SemanticInputPaths returns the normative duplicate-free semantic input order.
func SemanticInputPaths(project ProjectConfig) []string {
	candidates := []string{"gunte.toml"}
	if project.ContractFiles == nil {
		candidates = append(candidates, "contracts.toml")
	} else {
		candidates = append(candidates, project.ContractFiles...)
	}
	if project.Project.VersionFrom != "" {
		candidates = append(candidates, project.Project.VersionFrom)
	}
	candidates = append(candidates, project.Sources.Files...)
	seen := map[string]bool{}
	result := make([]string, 0, len(candidates))
	for _, path := range candidates {
		if !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}

// TargetIDs returns target IDs in declaration order.
func (c ProjectConfig) TargetIDs() []string {
	ids := make([]string, len(c.Targets))
	for i, target := range c.Targets {
		ids[i] = target.ID
	}
	return ids
}

type Project struct {
	ID          string
	Version     string
	VersionFrom string
}

type Sources struct {
	Files        []string
	ManagedRoots []string
	AllowFiles   []string
	AllowDirs    []string
}

type Term struct {
	Name   string
	Values []TargetValue
}

type BodyValue struct {
	Name string
	From string
}

type TargetValue struct {
	TargetID string
	Value    string
}

type Target struct {
	ID           string
	OutputRoot   string
	ManagedRoots []string
	AllowFiles   []string
	AllowDirs    []string
	Rules        []Rule
}

type Profile string

const (
	ProfileMarkdown      Profile = "markdown-v1"
	ProfileYAML          Profile = "markdown+yaml-frontmatter-v1"
	ProfileTOML          Profile = "toml-v1"
	ProfileJSON          Profile = "json-v1"
	ProfilePlainText     Profile = "plain-text-v1"
	ProfileMultilineText Profile = "multiline-text-v1"
)

type Rule struct {
	Match     string
	Path      string
	Profile   Profile
	Header    string
	Metadata  []MetadataEntry
	BodyField string
	ValueFrom string
}

type MetadataType string

const (
	MetadataString     MetadataType = "string"
	MetadataStringList MetadataType = "string_list"
	MetadataCommaList  MetadataType = "comma_list"
	MetadataPlainToken MetadataType = "plain_token"
)

type MetadataEntry struct {
	Field    string
	From     string
	Type     MetadataType
	Required bool
}

type ContractRegistry struct {
	Contracts []Contract
}

type PredicateKind string

const (
	PredicateRequires    PredicateKind = "requires"
	PredicateForbids     PredicateKind = "forbids"
	PredicateOccurrences PredicateKind = "occurrences"
	PredicateOrder       PredicateKind = "order"
	PredicateStructure   PredicateKind = "structure"
)

type StructureSubject string

const (
	StructureSourceFrontmatter StructureSubject = "source_frontmatter"
	StructureArtifact          StructureSubject = "artifact"
)

type StructureFormat string

const (
	StructureYAML StructureFormat = "yaml"
	StructureTOML StructureFormat = "toml"
	StructureJSON StructureFormat = "json"
)

type AssertionOp string

const (
	AssertExists      AssertionOp = "exists"
	AssertAbsent      AssertionOp = "absent"
	AssertCardinality AssertionOp = "cardinality"
	AssertEquals      AssertionOp = "equals"
	AssertExactKeys   AssertionOp = "exact_keys"
	AssertListOrder   AssertionOp = "list_order"
	AssertListSet     AssertionOp = "list_set"
)

type TypedValue = typeddata.Value
type TypedEntry = typeddata.Entry
type TypedKind = typeddata.Kind

const (
	TypedString = typeddata.String
	TypedInt    = typeddata.Int
	TypedBool   = typeddata.Bool
	TypedList   = typeddata.List
	TypedMap    = typeddata.Map
)

type StructureAssertion struct {
	Path  string
	Op    AssertionOp
	Value *TypedValue
	Count *int64
}

type Contract struct {
	ID           string
	Kind         PredicateKind
	Slice        string
	Pattern      string
	Before       string
	After        string
	AppliesTo    []string
	Subject      StructureSubject
	Paths        []string
	ExcludePaths []string
	Count        *int64
	Format       StructureFormat
	Assertions   []StructureAssertion
	Position     ContractPosition
}

// ContractPosition identifies the predicate declaration or key position in its source file.
// Line and Column are one-origin; Column counts bytes from the start of the line.
type ContractPosition struct {
	Path   string
	Line   int
	Column int
}
