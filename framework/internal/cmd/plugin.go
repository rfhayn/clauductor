package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/clauductor/clauductor/internal/plugin"
	"github.com/spf13/cobra"
)

var pluginRepo string

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Package the operating model as a Claude Code plugin (docs/plugin.md)",
}

var pluginBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build plugin/ and .claude-plugin/marketplace.json from template/",
	Long: `Assemble the Claude Code plugin from template/: the framework tier (skills, agents,
hooks, the workflow, the checks and scripts) with its paths rewritten to the plugin root, and
the project-owned files under plugin/scaffold/ for /clauductor:init. Writes plugin/ and
.claude-plugin/marketplace.json in the clauductor repository. Run it after any template change;
TestCommittedPluginIsCurrent fails until you do.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, err := findRepo(pluginRepo)
		if err != nil {
			return err
		}
		res, err := plugin.Build(plugin.Options{RepoRoot: repo, Version: Version})
		if err != nil {
			return err
		}
		fmt.Printf("Built %s/ (%d files, %d of them the project scaffold) and .claude-plugin/marketplace.json, version %s\n",
			plugin.PluginDir, len(res.Plugin), len(res.Scaffold), Version)
		return nil
	},
}

var pluginCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Fail if the committed plugin/ differs from a fresh build of template/",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, err := findRepo(pluginRepo)
		if err != nil {
			return err
		}
		diffs, err := PluginDrift(repo)
		if err != nil {
			return err
		}
		if len(diffs) > 0 {
			return fmt.Errorf("plugin/ is stale against template/ (run `clauductor plugin build`):\n  %s", strings.Join(diffs, "\n  "))
		}
		fmt.Println("plugin/ matches template/")
		return nil
	},
}

func init() {
	pluginCmd.PersistentFlags().StringVar(&pluginRepo, "repo", "", "the clauductor repository (default: the git toplevel of the working directory)")
	pluginCmd.AddCommand(pluginBuildCmd, pluginCheckCmd)
	rootCmd.AddCommand(pluginCmd)
}

// findRepo is the clauductor repository: the flag, else the working directory's git toplevel,
// which must hold template/.
func findRepo(flag string) (string, error) {
	repo := flag
	if repo == "" {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return "", fmt.Errorf("not in a git repository; pass --repo <clauductor checkout>")
		}
		repo = strings.TrimSpace(string(out))
	}
	if _, err := os.Stat(filepath.Join(repo, "template", ".claude")); err != nil {
		return "", fmt.Errorf("%s is not a clauductor checkout (no template/.claude)", repo)
	}
	return repo, nil
}

// PluginDrift builds the plugin into a temporary directory and lists every path where it and the
// committed plugin/ (or the committed marketplace.json) differ, in content or executable bit.
func PluginDrift(repo string) ([]string, error) {
	tmp, err := os.MkdirTemp("", "clauductor-plugin-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "plugin")
	if _, err := plugin.Build(plugin.Options{RepoRoot: repo, OutDir: out, Version: Version}); err != nil {
		return nil, err
	}
	fresh, err := snapshot(out)
	if err != nil {
		return nil, err
	}
	committed, err := snapshot(filepath.Join(repo, plugin.PluginDir))
	if err != nil {
		return nil, err
	}
	var diffs []string
	for p, f := range fresh {
		c, ok := committed[p]
		switch {
		case !ok:
			diffs = append(diffs, "missing: "+p)
		case !bytes.Equal(c.data, f.data):
			diffs = append(diffs, "differs: "+p)
		case c.exec != f.exec:
			diffs = append(diffs, "mode differs: "+p)
		}
	}
	for p := range committed {
		if _, ok := fresh[p]; !ok {
			diffs = append(diffs, "stale: "+p)
		}
	}
	mk, err := plugin.MarketplaceJSON()
	if err != nil {
		return nil, err
	}
	have, err := os.ReadFile(filepath.Join(repo, ".claude-plugin", "marketplace.json"))
	if err != nil || !bytes.Equal(have, mk) {
		diffs = append(diffs, "differs: .claude-plugin/marketplace.json")
	}
	sort.Strings(diffs)
	return diffs, nil
}

type snapFile struct {
	data []byte
	exec bool
}

func snapshot(root string) (map[string]snapFile, error) {
	m := map[string]snapFile{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == root {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		m[filepath.ToSlash(rel)] = snapFile{data: data, exec: info.Mode().Perm()&0o100 != 0}
		return nil
	})
	return m, err
}
