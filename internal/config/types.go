package config

// Diagnostic describes a correctable configuration problem.
type Diagnostic struct {
	Path    string
	Line    int
	Column  int
	Message string
}

// ProjectConfig is the validated project configuration in declaration order.
type ProjectConfig struct {
	SpecVersion int
	Project     Project
	Sources     Sources
	Terms       []Term
	Targets     []Target
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
	ID      string
	Version string
}

type Sources struct {
	Files []string
}

type Term struct {
	Name   string
	Values []TargetValue
}

type TargetValue struct {
	TargetID string
	Value    string
}

type Target struct {
	ID         string
	OutputRoot string
	Rules      []Rule
}

type Profile string

const (
	ProfileMarkdown  Profile = "markdown-v1"
	ProfileYAML      Profile = "markdown+yaml-frontmatter-v1"
	ProfileTOML      Profile = "toml-v1"
	ProfileJSON      Profile = "json-v1"
	ProfilePlainText Profile = "plain-text-v1"
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
	PredicateRequires PredicateKind = "requires"
	PredicateForbids  PredicateKind = "forbids"
	PredicateOrder    PredicateKind = "order"
)

type Contract struct {
	ID        string
	Kind      PredicateKind
	Slice     string
	Pattern   string
	Before    string
	After     string
	AppliesTo []string
}
