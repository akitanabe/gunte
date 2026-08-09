package gunte_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflow struct {
	Name        string            `yaml:"name"`
	On          workflowTriggers  `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]job    `yaml:"jobs"`
}

type workflowTriggers struct {
	Push struct {
		Branches []string `yaml:"branches"`
		Tags     []string `yaml:"tags"`
	} `yaml:"push"`
	PullRequest *struct{} `yaml:"pull_request"`
}

type job struct {
	Needs       string            `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	RunsOn      string            `yaml:"runs-on"`
	Steps       []workflowStep    `yaml:"steps"`
}

type workflowStep struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func TestCIWorkflowTestsMainAndPullRequests(t *testing.T) {
	ci := readWorkflow(t, ".github/workflows/ci.yml")
	assertEqualStrings(t, ci.On.Push.Branches, []string{"main"})
	if ci.On.PullRequest == nil {
		t.Fatal("pull_request trigger is missing")
	}
	if ci.Permissions["contents"] != "read" {
		t.Fatalf("contents permission = %q, want read", ci.Permissions["contents"])
	}
	assertGoTestJob(t, ci.Jobs["test"])
}

func TestReleaseWorkflowPackagesVerifiedTagAndPublishesRetryably(t *testing.T) {
	release := readWorkflow(t, ".github/workflows/release.yml")
	assertEqualStrings(t, release.On.Push.Tags, []string{"v*"})
	if release.Permissions["contents"] != "read" {
		t.Fatalf("default contents permission = %q, want read", release.Permissions["contents"])
	}
	assertGoTestJob(t, release.Jobs["test"])

	publish := release.Jobs["release"]
	if publish.Needs != "test" || publish.RunsOn != "ubuntu-latest" {
		t.Fatalf("release job needs=%q runs-on=%q", publish.Needs, publish.RunsOn)
	}
	if publish.Permissions["contents"] != "write" {
		t.Fatalf("release contents permission = %q, want write", publish.Permissions["contents"])
	}
	assertUses(t, publish, "actions/checkout@v6")
	assertUses(t, publish, "actions/setup-go@v7")
	assertRunContains(t, publish, "scripts/package-release.sh")
	assertRunContains(t, publish, "gh release create", "--draft", "--verify-tag", "--generate-notes")
	assertRunContains(t, publish, "gh release upload", "--clobber", "dist/*")
	assertRunContains(t, publish, "gh release edit", "--draft=false")
	for _, step := range publish.Steps {
		if strings.Contains(step.Run, "gh release") {
			if step.Env["GH_TOKEN"] != "${{ github.token }}" {
				t.Fatalf("release step %q does not use github.token", step.Name)
			}
			if step.If != "${{ !env.ACT }}" {
				t.Fatalf("release step %q is not disabled under act", step.Name)
			}
		}
	}
}

func readWorkflow(t *testing.T, path string) workflow {
	t.Helper()
	var result workflow
	if err := yaml.Unmarshal(readFile(t, filepath.FromSlash(path)), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertGoTestJob(t *testing.T, actual job) {
	t.Helper()
	if actual.RunsOn != "ubuntu-latest" {
		t.Fatalf("test runs-on = %q, want ubuntu-latest", actual.RunsOn)
	}
	assertUses(t, actual, "actions/checkout@v6")
	setup := assertUses(t, actual, "actions/setup-go@v7")
	if setup.With["go-version-file"] != "go.mod" {
		t.Fatalf("go-version-file = %q, want go.mod", setup.With["go-version-file"])
	}
	assertRunContains(t, actual, "go test -count=1 ./...")
	assertRunContains(t, actual, "go vet ./...")
}

func assertUses(t *testing.T, actual job, action string) workflowStep {
	t.Helper()
	for _, step := range actual.Steps {
		if step.Uses == action {
			return step
		}
	}
	t.Fatalf("job does not use %s", action)
	return workflowStep{}
}

func assertRunContains(t *testing.T, actual job, fragments ...string) {
	t.Helper()
	for _, step := range actual.Steps {
		matches := true
		for _, fragment := range fragments {
			matches = matches && strings.Contains(step.Run, fragment)
		}
		if matches {
			return
		}
	}
	t.Fatalf("job has no run step containing %q", fragments)
}
