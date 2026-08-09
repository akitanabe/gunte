package gunte_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

func TestReleaseBuildProducesOneBinaryForEachSupportedPlatform(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "dist")
	command := exec.Command("./scripts/build-release.sh", outputDir)
	command.Env = append(os.Environ(),
		"GOCACHE="+filepath.Join(t.TempDir(), "go-cache"),
		"GOPROXY=off",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build release binaries: %v\n%s", err, output)
	}

	wantMagic := map[string][]byte{
		"gunte-darwin-amd64":      {0xcf, 0xfa, 0xed, 0xfe},
		"gunte-darwin-arm64":      {0xcf, 0xfa, 0xed, 0xfe},
		"gunte-linux-amd64":       {0x7f, 'E', 'L', 'F'},
		"gunte-linux-arm64":       {0x7f, 'E', 'L', 'F'},
		"gunte-windows-amd64.exe": {'M', 'Z'},
		"gunte-windows-arm64.exe": {'M', 'Z'},
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected directory %q", entry.Name())
		}
		gotNames = append(gotNames, entry.Name())
		binary, err := os.ReadFile(filepath.Join(outputDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if magic, ok := wantMagic[entry.Name()]; !ok {
			t.Errorf("unexpected artifact %q", entry.Name())
		} else if !bytes.HasPrefix(binary, magic) {
			t.Errorf("%s does not have the expected executable format", entry.Name())
		}
	}
	sort.Strings(gotNames)
	wantNames := make([]string, 0, len(wantMagic))
	for name := range wantMagic {
		wantNames = append(wantNames, name)
	}
	sort.Strings(wantNames)
	assertEqualStrings(t, gotNames, wantNames)
}
