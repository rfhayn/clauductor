package template

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

// FileDiff represents a difference between template and project file.
type FileDiff struct {
	Path   string // relative path
	Status string // "new", "modified", "outdated"
}

// FindDiffs compares template files against project files.
func FindDiffs(targetDir string) ([]FileDiff, error) {
	tmplPath, err := TemplatePath()
	if err != nil {
		return nil, err
	}

	var diffs []FileDiff

	// Only compare skills and specific doc templates, not user content
	comparePaths := []string{
		".claude/skills",
		".claude/agents",
	}

	for _, cp := range comparePaths {
		srcDir := filepath.Join(tmplPath, cp)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}

		filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			relPath, _ := filepath.Rel(tmplPath, path)
			destPath := filepath.Join(targetDir, relPath)

			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				diffs = append(diffs, FileDiff{Path: relPath, Status: "new"})
				return nil
			}

			// Compare content hashes
			srcHash, _ := fileHash(path)
			destHash, _ := fileHash(destPath)
			if srcHash != destHash {
				diffs = append(diffs, FileDiff{Path: relPath, Status: "modified"})
			}

			return nil
		})
	}

	return diffs, nil
}

// ApplyUpdate copies a single template file to the target directory.
func ApplyUpdate(targetDir string, diff FileDiff) error {
	tmplPath, err := TemplatePath()
	if err != nil {
		return err
	}

	srcPath := filepath.Join(tmplPath, diff.Path)
	destPath := filepath.Join(targetDir, diff.Path)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	return copyFile(srcPath, destPath)
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
