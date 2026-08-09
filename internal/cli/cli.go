// Package cli contains the command-line grammar and its user-facing help.
// Parsing and rendering are calculations: they do not inspect the project or
// perform filesystem actions.
package cli

// UsageMessage is kept stable for callers that need to report a usage error.
const UsageMessage = "usage: gunte emit|check [--target ID] | gunte lock"

// Command identifies an executable project operation.
type Command string

const (
	CommandEmit  Command = "emit"
	CommandCheck Command = "check"
	CommandLock  Command = "lock"
)

// HelpTopic identifies one rendered help document.
type HelpTopic string

const (
	HelpRoot  HelpTopic = "root"
	HelpEmit  HelpTopic = "emit"
	HelpCheck HelpTopic = "check"
	HelpLock  HelpTopic = "lock"
)

// ExecuteRequest is the parsed data passed from the CLI boundary to project
// execution. Target is empty when all targets are selected.
type ExecuteRequest struct {
	Command Command
	Target  string
}

// Valid reports whether an execute request satisfies the command boundary's
// invariant. Target spelling and existence belong to project configuration;
// this boundary only enforces that lock has no target.
func (request ExecuteRequest) Valid() bool {
	switch request.Command {
	case CommandEmit, CommandCheck:
		return true
	case CommandLock:
		return request.Target == ""
	default:
		return false
	}
}

// ParseKind identifies the parser's next action.
type ParseKind uint8

const (
	ParseUsageError ParseKind = iota
	ParseHelp
	ParseExecute
)

// ParseResult is the parser output. Only the fields relevant to Kind are set.
type ParseResult struct {
	Kind    ParseKind
	Topic   HelpTopic
	Request ExecuteRequest
	Message string
}

// Parse turns argv into either a help topic, an execute request, or a usage
// error. It intentionally does not validate project-specific target existence.
func Parse(args []string) ParseResult {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "--help", "-h":
		if len(args) == 1 {
			return helpResult(HelpRoot)
		}
		return usageError()
	case "help":
		if len(args) == 1 {
			return helpResult(HelpRoot)
		}
		if len(args) == 2 {
			switch args[1] {
			case string(CommandEmit):
				return helpResult(HelpEmit)
			case string(CommandCheck):
				return helpResult(HelpCheck)
			case string(CommandLock):
				return helpResult(HelpLock)
			}
		}
		return usageError()
	case string(CommandEmit), string(CommandCheck):
		command := Command(args[0])
		if len(args) == 1 {
			return executeResult(ExecuteRequest{Command: command})
		}
		if len(args) == 2 && args[1] == "--help" {
			if command == CommandEmit {
				return helpResult(HelpEmit)
			}
			return helpResult(HelpCheck)
		}
		if len(args) == 3 && args[1] == "--target" && nonEmptyTargetArgument(args[2]) {
			return executeResult(ExecuteRequest{Command: command, Target: args[2]})
		}
		return usageError()
	case string(CommandLock):
		if len(args) == 1 {
			return executeResult(ExecuteRequest{Command: CommandLock})
		}
		if len(args) == 2 && args[1] == "--help" {
			return helpResult(HelpLock)
		}
		return usageError()
	default:
		return usageError()
	}
}

func nonEmptyTargetArgument(value string) bool {
	return value != ""
}

func usageError() ParseResult {
	return ParseResult{Kind: ParseUsageError, Message: UsageMessage}
}

func helpResult(topic HelpTopic) ParseResult {
	return ParseResult{Kind: ParseHelp, Topic: topic}
}

func executeResult(request ExecuteRequest) ParseResult {
	return ParseResult{Kind: ParseExecute, Request: request}
}

// Render returns deterministic help text for a topic without reading project
// files. The text is deliberately explicit about side effects and validation
// scope so users can choose a command before entering a project directory.
func Render(topic HelpTopic) string {
	switch topic {
	case HelpEmit:
		return emitHelp
	case HelpCheck:
		return checkHelp
	case HelpLock:
		return lockHelp
	default:
		return rootHelp
	}
}

const rootHelp = `Gunte compiles a project’s canonical sources into deterministic prompt artifacts and validates their contracts.

Usage:
  gunte <command> [options]

Commands:
  emit    validate the project, generate artifacts, and write them
  check   validate and compare generated artifacts without writing
  lock    validate a v2 project and update gunte.lock.json
  help    show this help or help for one command

Options:
  -h, --help    show help and exit successfully

Working directory:
  Commands use the current working directory as the project root (cwd=project root).
  Help is complete before cwd/project loading and succeeds outside a project.

Exit status:
  0  success
  1  project validation, contract, output, or I/O failure
  2  command-line usage error

Command help:
  gunte help <command>
  gunte <command> --help
`

const emitHelp = `Usage: gunte emit [--target ID]

Purpose:
  Validate the complete project and generate deterministic artifacts.

Input and validation:
  Reads gunte.toml, the selected contract registry files configured there,
  configured version/source files, and all target rules. Configuration, source,
  registry, generation, and contract validation cover the full project before
  any artifact is written.

Writes and side effects:
  After every validation succeeds, writes generated artifacts. emit never
  updates gunte.lock.json. Existing stale files are not removed.

Options:
  --target ID   limit artifact generation and target-specific checks to target ID;
                project configuration and full-project validation still run.
  --help        show this help and exit successfully.

Success and failure:
  Exit 0 means all validation passed and writes completed.
  Exit 1 means validation, contract, or write I/O failed; failed validation
  performs no artifact writes. Exit 2 means invalid command-line arguments.

Example:
  gunte emit
  gunte emit --target claude
`

const checkHelp = `Usage: gunte check [--target ID]

Purpose:
  Validate the complete project and compare generated artifact bytes with files
  already present on disk.

Input and validation:
  Reads gunte.toml, the selected contract registry files configured there,
  configured version/source files, and all target rules, then performs
  full-project validation. With v2, check also validates managed inventory and
  reports a missing or mismatched full gunte.lock.json; target ID limits
  target-specific artifact comparison while full-project checks remain enabled.

Writes and side effects:
  check is read-only: it never calls the artifact writer, removes stale files,
  or updates the lock.

Options:
  --target ID   limit artifact comparison and target-specific checks to target ID;
                project configuration and full-project validation still run.
  --help        show this help and exit successfully.

Success and failure:
  Exit 0 means validation passed and every expected artifact (and v2 lock and
  inventory) matched. Exit 1 means validation, artifact mismatch, v2 inventory,
  v2 lock mismatch, or read I/O failed. Exit 2 means invalid arguments.

Example:
  gunte check
  gunte check --target codex
`

const lockHelp = `Usage: gunte lock

Purpose:
  Validate a Spec-Version 2 project and record its canonical semantic lock.

Input and validation:
  Reads gunte.toml, the selected contract registry files configured there,
  configured version/source files, and all targets, then performs full-project
  configuration, source, registry, generation, and contract validation.

Writes and side effects:
  lock is v2-only and has no target option. After validation succeeds, it
  updates gunte.lock.json atomically. It does not generate artifacts.

Options:
  --help        show this help and exit successfully.
  A target option is not supported by lock.

Success and failure:
  Exit 0 means the v2 lock was updated. Exit 1 means the project is not v2 or
  validation/write I/O failed. Exit 2 means invalid command-line arguments.

Example:
  gunte lock
`
