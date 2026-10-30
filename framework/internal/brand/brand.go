// Package brand holds the product's name, in one place (OPS-101, OPS-104).
//
// The shell half is _brand_env in template/.claude/lib/conf.sh; TestShellBrandMatches
// holds the two to the same value.
package brand

// Name is the product's name: the CLI, the dot directories, the panel's host name, its
// hook tag, its launchd label and its tmux options are all spelled from it.
const Name = "stridal"

// Dir is the dot directory a name keeps its files in, under the home directory and
// under a project root: ".stridal".
func Dir(name string) string { return "." + name }

// HookTag marks a hook entry in ~/.claude/settings.json as a name's panel's own.
func HookTag(name string) string { return name + "-panel" }

// LaunchdLabel names a name's panel login agent.
func LaunchdLabel(name string) string { return "com." + name + ".panel" }

// TmuxOption is the session option a name's panel tags a lane with: "@stridal_type".
func TmuxOption(name, key string) string { return "@" + name + "_" + key }

// InstallMarker is the file `install` writes in a repository it put the model in.
func InstallMarker(name string) string { return ".claude/" + name + "-template" }

// PluginMarker is the file the plugin's init writes in a repository that runs the model
// from the plugin.
func PluginMarker(name string) string { return ".claude/" + name + "-plugin" }
