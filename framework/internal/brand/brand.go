// Package brand holds the product's name, in one place, for the rename (OPS-101, OPS-104).
//
// Name is the name now; the rename sweep changes it here. LegacyName is the name before
// the rename and never changes: a machine or a repository set up under it keeps its
// files, markers, hooks and tmux options until it migrates, so every reader of one of
// those tries each of Names in turn, Name first. While the two are equal (rename step 3)
// Names has one entry, and every reader does exactly what it did before.
//
// The shell half is _brand_env in template/.claude/lib/conf.sh; TestShellBrandMatches
// holds the two to the same values.
package brand

import "os"

// Name is the product's name: the CLI, the dot directories, the panel's host name, its
// hook tag, its launchd label and its tmux options are all spelled from it.
const Name = "clauductor"

// LegacyName is the name an earlier install wrote everything under. It never changes.
const LegacyName = "clauductor"

// Names is the order every fallback reader tries: Name, then LegacyName when it differs.
func Names() []string {
	if Name == LegacyName {
		return []string{Name}
	}
	return []string{Name, LegacyName}
}

// Dir is the dot directory a name keeps its files in, under the home directory and
// under a project root: ".clauductor".
func Dir(name string) string { return "." + name }

// HookTag marks a hook entry in ~/.claude/settings.json as a name's panel's own.
func HookTag(name string) string { return name + "-panel" }

// LaunchdLabel names a name's panel login agent.
func LaunchdLabel(name string) string { return "com." + name + ".panel" }

// TmuxOption is the session option a name's panel tags a lane with: "@clauductor_type".
func TmuxOption(name, key string) string { return "@" + name + "_" + key }

// InstallMarker is the file `install` writes in a repository it put the model in.
func InstallMarker(name string) string { return ".claude/" + name + "-template" }

// PluginMarker is the file the plugin's init writes in a repository that runs the model
// from the plugin.
func PluginMarker(name string) string { return ".claude/" + name + "-plugin" }

// Existing returns the first of paths that exists, else the first: the new location,
// where a write goes when nothing was there before. When every path is the same one (the
// names are equal) it returns it without touching the disk.
func Existing(paths ...string) string {
	if len(paths) == 0 {
		return ""
	}
	same := true
	for _, p := range paths[1:] {
		same = same && p == paths[0]
	}
	if same {
		return paths[0]
	}
	for _, p := range paths {
		if _, err := os.Lstat(p); err == nil {
			return p
		}
	}
	return paths[0]
}
