// Package inventory contains the deterministic, filesystem-independent rules
// for comparing one managed scope snapshot with its expected and allowed paths.
package inventory

import (
	"sort"
	"strings"
)

// Kind is the observed filesystem entry kind. Symlink is deliberately kept as
// a leaf so that inventory comparison never follows directory symlinks.
type Kind string

const (
	KindFile    Kind = "file"
	KindDir     Kind = "directory"
	KindSymlink Kind = "symlink"
	KindOther   Kind = "other"
)

// Entry is one project-relative path observed below a managed root.
type Entry struct {
	Path string
	Kind Kind
}

// Scope describes one project-relative managed directory and its exceptions.
type Scope struct {
	Root       string
	AllowFiles []string
	AllowDirs  []string
}

// Diagnostic is a deterministic inventory mismatch at Path.
type Diagnostic struct {
	Path    string
	Message string
}

// Compare returns inventory mismatches for a single managed scope. Missing
// roots and missing allow entries are intentionally represented by no Entry
// and therefore do not produce diagnostics here.
func Compare(scope Scope, expected []string, entries []Entry) []Diagnostic {
	expectedSet := stringSet(expected)
	allowFileSet := stringSet(scope.AllowFiles)
	allowDirSet := stringSet(scope.AllowDirs)
	ancestors := make([]string, 0, len(expected)+len(scope.AllowFiles)+len(scope.AllowDirs))
	ancestors = append(ancestors, expected...)
	ancestors = append(ancestors, scope.AllowFiles...)
	ancestors = append(ancestors, scope.AllowDirs...)
	ordered := append([]Entry(nil), entries...)
	sort.SliceStable(ordered, func(first, second int) bool { return ordered[first].Path < ordered[second].Path })
	result := make([]Diagnostic, 0)
	for _, entry := range ordered {
		if !pathContains(scope.Root, entry.Path) {
			continue
		}
		if entry.Path == scope.Root && entry.Kind == KindDir {
			continue
		}
		if expectedSet[entry.Path] || allowFileSet[entry.Path] {
			if entry.Kind == KindFile || entry.Kind == KindSymlink {
				continue
			}
			result = append(result, mismatch(entry.Path, "expected file is not a file"))
			continue
		}
		if allowDirSet[entry.Path] {
			if entry.Kind == KindDir {
				continue
			}
			result = append(result, mismatch(entry.Path, "allow directory is not a directory"))
			continue
		}
		if underAllowDir(entry.Path, scope.AllowDirs) {
			continue
		}
		if entry.Kind == KindDir && ancestorOfAny(entry.Path, ancestors) {
			continue
		}
		result = append(result, mismatch(entry.Path, "path is outside expected and allowed inventory"))
	}
	return result
}

func mismatch(path, message string) Diagnostic {
	return Diagnostic{Path: path, Message: message}
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func underAllowDir(path string, dirs []string) bool {
	for _, dir := range dirs {
		if path != dir && pathContains(dir, path) {
			return true
		}
	}
	return false
}

func ancestorOfAny(path string, values []string) bool {
	for _, value := range values {
		if path != value && pathContains(path, value) {
			return true
		}
	}
	return false
}

func pathContains(root, value string) bool {
	return root == value || strings.HasPrefix(value, root+"/")
}
