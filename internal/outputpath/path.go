// Package outputpath calculates normalized project-relative output paths.
package outputpath

// Join combines a validated output root and a validated relative artifact
// path. A dot output root is the Spec-Version 2 repository-root sentinel.
func Join(root, relative string) string {
	if root == "." || root == "" {
		return relative
	}
	if relative == "" {
		return root
	}
	return root + "/" + relative
}
