package gunte_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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

func TestReleasePackageContainsSupportedBinariesAndMatchingChecksums(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "release")
	command := exec.Command("./scripts/package-release.sh", outputDir)
	command.Env = append(os.Environ(),
		"GOCACHE="+filepath.Join(t.TempDir(), "go-cache"),
		"GOPROXY=off",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("package release: %v\n%s", err, output)
	}

	wantArchives := map[string]string{
		"gunte-darwin-amd64.tar.gz": "gunte-darwin-amd64",
		"gunte-darwin-arm64.tar.gz": "gunte-darwin-arm64",
		"gunte-linux-amd64.tar.gz":  "gunte-linux-amd64",
		"gunte-linux-arm64.tar.gz":  "gunte-linux-arm64",
		"gunte-windows-amd64.zip":   "gunte-windows-amd64.exe",
		"gunte-windows-arm64.zip":   "gunte-windows-arm64.exe",
	}
	gotNames := regularFiles(t, outputDir)
	wantNames := []string{"SHA256SUMS"}
	for archive := range wantArchives {
		wantNames = append(wantNames, archive)
	}
	sort.Strings(wantNames)
	assertEqualStrings(t, gotNames, wantNames)

	for archive, binary := range wantArchives {
		assertEqualStrings(t, releaseArchiveEntries(t, filepath.Join(outputDir, archive)), []string{binary})
	}
	assertReleaseChecksums(t, outputDir, wantArchives)
}

func releaseArchiveEntries(t *testing.T, path string) []string {
	t.Helper()
	if strings.HasSuffix(path, ".zip") {
		reader, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		entries := make([]string, 0, len(reader.File))
		for _, file := range reader.File {
			entries = append(entries, file.Name)
		}
		return entries
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var entries []string
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return entries
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, header.Name)
	}
}

func assertReleaseChecksums(t *testing.T, outputDir string, archives map[string]string) {
	t.Helper()
	lines := nonemptyLines(readFile(t, filepath.Join(outputDir, "SHA256SUMS")))
	if len(lines) != len(archives) {
		t.Fatalf("checksum lines = %d, want %d", len(lines), len(archives))
	}
	gotArchives := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid checksum line %q", line)
		}
		archive := fields[1]
		if _, ok := archives[archive]; !ok {
			t.Fatalf("unexpected checksum target %q", archive)
		}
		gotArchives = append(gotArchives, archive)
		digest := sha256.Sum256(readFile(t, filepath.Join(outputDir, archive)))
		if got, want := fields[0], hex.EncodeToString(digest[:]); got != want {
			t.Errorf("checksum for %s = %s, want %s", archive, got, want)
		}
	}
	sort.Strings(gotArchives)
	wantArchives := make([]string, 0, len(archives))
	for archive := range archives {
		wantArchives = append(wantArchives, archive)
	}
	sort.Strings(wantArchives)
	assertEqualStrings(t, gotArchives, wantArchives)
}
