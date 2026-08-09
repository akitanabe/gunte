package gunte_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevcontainerProvidesPinnedActWithHostDockerWorkspaceAccess(t *testing.T) {
	var config struct {
		Image           string                    `json:"image"`
		Features        map[string]map[string]any `json:"features"`
		WorkspaceFolder string                    `json:"workspaceFolder"`
		WorkspaceMount  string                    `json:"workspaceMount"`
	}
	path := filepath.Join(".devcontainer", "devcontainer.json")
	if err := json.Unmarshal(readFile(t, path), &config); err != nil {
		t.Fatal(err)
	}
	if config.Image != "mcr.microsoft.com/devcontainers/go:1.26-bookworm" {
		t.Fatalf("devcontainer image = %q", config.Image)
	}
	if _, ok := config.Features["ghcr.io/devcontainers/features/docker-outside-of-docker:1"]; !ok {
		t.Fatal("docker-outside-of-docker feature is missing")
	}
	act, ok := config.Features["ghcr.io/devcontainers-extra/features/act:1"]
	if !ok || act["version"] != "0.2.88" {
		t.Fatalf("act feature = %#v", act)
	}
	if config.WorkspaceFolder != "${localWorkspaceFolder}" || !strings.Contains(config.WorkspaceMount, "target=${localWorkspaceFolder}") {
		t.Fatalf("workspace folder=%q mount=%q", config.WorkspaceFolder, config.WorkspaceMount)
	}

	actConfig := string(readFile(t, ".actrc"))
	if !strings.Contains(actConfig, "-P ubuntu-latest=catthehacker/ubuntu:act-latest") {
		t.Fatalf(".actrc does not select the medium ubuntu-latest runner: %q", actConfig)
	}
}
