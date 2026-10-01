package template

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TemplatePath returns the path to the template directory.
// It looks for the template relative to the installed framework location.
func TemplatePath() (string, error) {
	// Check CLAUDUCTOR_FRAMEWORK env var first
	if envPath := os.Getenv("CLAUDUCTOR_FRAMEWORK"); envPath != "" {
		tmplPath := filepath.Join(envPath, "template")
		if _, err := os.Stat(tmplPath); err == nil {
			return tmplPath, nil
		}
	}

	// Check common install locations
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %w", err)
	}

	candidates := []string{
		filepath.Join(home, "clauductor", "template"),
		filepath.Join(home, "Development", "clauductor", "template"),
		filepath.Join(home, "claude-dev-framework", "template"),
		filepath.Join(home, "Development", "claude-dev-framework", "template"),
		filepath.Join(home, ".clauductor", "template"),
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("could not find Clauductor template directory — set CLAUDUCTOR_FRAMEWORK env var to framework repo root")
}

// ListTemplateFiles returns every template file's path relative to the template, with forward
// slashes, sorted. Finder litter and lane worktrees (a checkout under template/.claude/worktrees)
// are not template files.
func ListTemplateFiles() ([]string, error) {
	tmplPath, err := TemplatePath()
	if err != nil {
		return nil, err
	}
	return listDir(tmplPath)
}

func listDir(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "worktrees" && filepath.Base(filepath.Dir(p)) == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

// CopyTemplate copies all template files to the target directory.
func CopyTemplate(targetDir string) error {
	return CopyTemplateWithSkips(targetDir, nil)
}

// CopyTemplateWithSkips copies template files, skipping specified relative paths.
func CopyTemplateWithSkips(targetDir string, skipFiles map[string]bool) error {
	tmplPath, err := TemplatePath()
	if err != nil {
		return err
	}

	return filepath.WalkDir(tmplPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(tmplPath, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Check if this file should be skipped
		if skipFiles != nil && skipFiles[relPath] {
			return nil
		}

		return copyFile(path, destPath)
	})
}

// FindConflicts returns relative paths of template files that already exist in targetDir.
func FindConflicts(targetDir string) ([]string, error) {
	tmplPath, err := TemplatePath()
	if err != nil {
		return nil, err
	}

	var conflicts []string
	filepath.WalkDir(tmplPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(tmplPath, path)
		destPath := filepath.Join(targetDir, relPath)

		if _, err := os.Stat(destPath); err == nil {
			conflicts = append(conflicts, relPath)
		}
		return nil
	})

	return conflicts, nil
}

// FileDiff represents a difference between a template file and the project's copy.
type FileDiff struct {
	Path   string // template-relative path
	Dest   string // where the project keeps it (Path unless project.conf moves it)
	Status string // "new" or "modified"
}

// FindDiffs lists what `update` would refresh: every framework-tier file (Classify over the whole
// template, so a new framework script cannot be left out the way a hand list left five, B3) that
// is missing or differs, at the path the project's config puts it, plus the agents the project
// lacks or has changed (doc tier, but offered for review as update always did). settings.json is
// not here: it is merged, not copied (PlanSettings).
func FindDiffs(targetDir string) ([]FileDiff, error) {
	tmplPath, err := TemplatePath()
	if err != nil {
		return nil, err
	}
	files, err := listDir(tmplPath)
	if err != nil {
		return nil, err
	}
	conf, err := LoadConf(targetDir, tmplPath)
	if err != nil {
		return nil, err
	}
	pm, err := conf.Mapping(files)
	if err != nil {
		return nil, err
	}
	var diffs []FileDiff
	for _, rel := range files {
		if !UpdateRefreshes(rel) {
			continue
		}
		dest, _, skip := pm.Resolve(rel)
		if skip != "" {
			continue
		}
		destPath := filepath.Join(targetDir, filepath.FromSlash(dest))
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			diffs = append(diffs, FileDiff{Path: rel, Dest: dest, Status: "new"})
			continue
		}
		if !FilesEqual(filepath.Join(tmplPath, filepath.FromSlash(rel)), destPath) {
			diffs = append(diffs, FileDiff{Path: rel, Dest: dest, Status: "modified"})
		}
	}
	return diffs, nil
}

// UpdateRefreshes says whether `update` compares rel file by file.
func UpdateRefreshes(rel string) bool {
	return Classify(rel) == TierFramework || strings.HasPrefix(rel, ".claude/agents/")
}

// ApplyUpdate copies a single template file to where the project keeps it.
func ApplyUpdate(targetDir string, diff FileDiff) error {
	dest := diff.Dest
	if dest == "" {
		dest = diff.Path
	}
	return CopyFile(targetDir, diff.Path, dest)
}

// CopyFile copies the template file rel to dest (relative to targetDir).
func CopyFile(targetDir, rel, dest string) error {
	tmplPath, err := TemplatePath()
	if err != nil {
		return err
	}
	return copyFile(filepath.Join(tmplPath, filepath.FromSlash(rel)), filepath.Join(targetDir, filepath.FromSlash(dest)))
}

// FilesEqual says whether two files have the same content.
func FilesEqual(a, b string) bool {
	ha, err1 := fileHash(a)
	hb, err2 := fileHash(b)
	return err1 == nil && err2 == nil && ha == hb
}

func copyFile(src, dst string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// StripComments removes lines starting with # from content (for comparison).
func StripComments(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
