package cli

import (
	"strings"
	"testing"
)

func TestParseHelpEntrypoints(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		topic HelpTopic
	}{
		{name: "long root", args: []string{"--help"}, topic: HelpRoot},
		{name: "short root", args: []string{"-h"}, topic: HelpRoot},
		{name: "help command", args: []string{"help"}, topic: HelpRoot},
		{name: "help emit", args: []string{"help", "emit"}, topic: HelpEmit},
		{name: "help check", args: []string{"help", "check"}, topic: HelpCheck},
		{name: "help lock", args: []string{"help", "lock"}, topic: HelpLock},
		{name: "emit option", args: []string{"emit", "--help"}, topic: HelpEmit},
		{name: "check option", args: []string{"check", "--help"}, topic: HelpCheck},
		{name: "lock option", args: []string{"lock", "--help"}, topic: HelpLock},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Parse(test.args)
			if result.Kind != ParseHelp || result.Topic != test.topic {
				t.Fatalf("Parse(%v) = %#v, want help topic %q", test.args, result, test.topic)
			}
		})
	}
}

func TestParseExecuteRequests(t *testing.T) {
	tests := []struct {
		args    []string
		command Command
		target  string
	}{
		{args: []string{"emit"}, command: CommandEmit},
		{args: []string{"check"}, command: CommandCheck},
		{args: []string{"lock"}, command: CommandLock},
		{args: []string{"emit", "--target", "claude"}, command: CommandEmit, target: "claude"},
		{args: []string{"check", "--target", "codex"}, command: CommandCheck, target: "codex"},
		{args: []string{"emit", "--target", "Bad"}, command: CommandEmit, target: "Bad"},
		{args: []string{"emit", "--target", "--help"}, command: CommandEmit, target: "--help"},
	}
	for _, test := range tests {
		result := Parse(test.args)
		want := ExecuteRequest{Command: test.command, Target: test.target}
		if result.Kind != ParseExecute || result.Request != want {
			t.Errorf("Parse(%v) = %#v, want execute %#v", test.args, result, want)
		}
	}
}

func TestParseRejectsUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"unknown"},
		{"emit", "--unknown"},
		{"emit", "-h"},
		{"emit", "--target"},
		{"emit", "--target", ""},
		{"check", "--target", ""},
		{"emit", "--target", "one", "extra"},
		{"lock", "--target", "one"},
		{"lock", "-h"},
		{"help", "unknown"},
		{"help", "emit", "extra"},
		{"--help", "extra"},
		{"emit", "--help", "extra"},
	} {
		result := Parse(args)
		if result.Kind != ParseUsageError || result.Message != UsageMessage {
			t.Errorf("Parse(%v) = %#v, want usage error %q", args, result, UsageMessage)
		}
	}
}

func TestExecuteRequestValidatesCommandAndLockTargetInvariant(t *testing.T) {
	tests := []struct {
		name    string
		request ExecuteRequest
		valid   bool
	}{
		{name: "emit all targets", request: ExecuteRequest{Command: CommandEmit}, valid: true},
		{name: "check target", request: ExecuteRequest{Command: CommandCheck, Target: "claude"}, valid: true},
		{name: "lock without target", request: ExecuteRequest{Command: CommandLock}, valid: true},
		{name: "unknown command", request: ExecuteRequest{Command: Command("unknown")}},
		{name: "lock target", request: ExecuteRequest{Command: CommandLock, Target: "claude"}},
		{name: "arbitrary target is project data", request: ExecuteRequest{Command: CommandEmit, Target: "Claude"}, valid: true},
		{name: "target shape is project data", request: ExecuteRequest{Command: CommandCheck, Target: "a12345678901234567890123456789012"}, valid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.request.Valid(); got != test.valid {
				t.Fatalf("ExecuteRequest.Valid() = %t, want %t for %#v", got, test.valid, test.request)
			}
		})
	}
}

func TestRenderHelpDescribesObservableCLIContract(t *testing.T) {
	root := Render(HelpRoot)
	for _, phrase := range []string{
		"Gunte",
		"gunte <command> [options]",
		"emit",
		"check",
		"lock",
		"help",
		"-h, --help",
		"project root",
		"Exit status",
		"0  success",
		"1  project",
		"2  command-line",
		"gunte help <command>",
		"compiles",
		"artifacts",
		"contracts",
		"validate the project, generate artifacts, and write them",
		"validate and compare generated artifacts without writing",
		"validate a v2 project and update gunte.lock.json",
		"show this help or help for one command",
	} {
		if !strings.Contains(root, phrase) {
			t.Errorf("root help does not contain %q:\n%s", phrase, root)
		}
	}
	for topic, phrases := range map[HelpTopic][]string{
		HelpEmit: {
			"Usage: gunte emit [--target ID]", "Purpose", "gunte.toml", "registry files",
			"before", "artifact is written", "--target ID", "full-project validation", "Success", "Failure", "Example",
		},
		HelpCheck: {
			"Usage: gunte check [--target ID]", "Purpose", "gunte.toml", "registry files", "read-only", "artifact",
			"inventory", "lock mismatch", "full-project validation", "--target ID", "Success", "Failure", "Example",
		},
		HelpLock: {
			"Usage: gunte lock", "Purpose", "v2-only", "no target", "gunte.lock.json",
			"registry files", "after validation succeeds", "Success", "Failure", "Example",
		},
	} {
		help := Render(topic)
		for _, phrase := range phrases {
			if !strings.Contains(strings.ToLower(help), strings.ToLower(phrase)) {
				t.Errorf("%q help does not contain %q:\n%s", topic, phrase, help)
			}
		}
	}
}
