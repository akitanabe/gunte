package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type artifact struct {
	binary  string
	archive string
}

var artifacts = []artifact{
	{binary: "gunte-darwin-amd64", archive: "gunte-darwin-amd64.tar.gz"},
	{binary: "gunte-darwin-arm64", archive: "gunte-darwin-arm64.tar.gz"},
	{binary: "gunte-linux-amd64", archive: "gunte-linux-amd64.tar.gz"},
	{binary: "gunte-linux-arm64", archive: "gunte-linux-arm64.tar.gz"},
	{binary: "gunte-windows-amd64.exe", archive: "gunte-windows-amd64.zip"},
	{binary: "gunte-windows-arm64.exe", archive: "gunte-windows-arm64.zip"},
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: package-release BINARY_DIR OUTPUT_DIR")
		os.Exit(2)
	}
	if err := packageRelease(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func packageRelease(binaryDir, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	checksums := make([]byte, 0, len(artifacts)*100)
	for _, artifact := range artifacts {
		binaryPath := filepath.Join(binaryDir, artifact.binary)
		archivePath := filepath.Join(outputDir, artifact.archive)
		var err error
		if filepath.Ext(artifact.archive) == ".zip" {
			err = writeZip(binaryPath, archivePath, artifact.binary)
		} else {
			err = writeTarGzip(binaryPath, archivePath, artifact.binary)
		}
		if err != nil {
			return fmt.Errorf("package %s: %w", artifact.binary, err)
		}
		archiveBytes, err := os.ReadFile(archivePath)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(archiveBytes)
		checksums = fmt.Appendf(checksums, "%x  %s\n", digest, artifact.archive)
	}
	return os.WriteFile(filepath.Join(outputDir, "SHA256SUMS"), checksums, 0o644)
}

func writeTarGzip(binaryPath, archivePath, name string) error {
	binary, mode, err := readBinary(binaryPath)
	if err != nil {
		return err
	}
	archive, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: int64(mode), Size: int64(len(binary))}); err != nil {
		return err
	}
	if _, err := tarWriter.Write(binary); err != nil {
		return err
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	return archive.Close()
}

func writeZip(binaryPath, archivePath, name string) error {
	binary, mode, err := readBinary(binaryPath)
	if err != nil {
		return err
	}
	archive, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	zipWriter := zip.NewWriter(archive)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	entry, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	if _, err := entry.Write(binary); err != nil {
		return err
	}
	if err := zipWriter.Close(); err != nil {
		return err
	}
	return archive.Close()
}

func readBinary(path string) ([]byte, os.FileMode, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	binary, err := io.ReadAll(file)
	return binary, info.Mode().Perm(), err
}
