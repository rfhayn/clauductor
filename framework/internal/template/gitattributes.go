package template

import "strings"

// GitattributesPath is the line-ending rules file the template ships. Git for Windows defaults to
// core.autocrlf=true, which checks every shell script out with CRLF unless the repository says
// otherwise, and a hook, check or gate script with CRLF fails in sh (`$'\r': command not found`).
// So the rules reach every project, new or existing: install creates or merges the file, and
// update merges it (an existing project gains the rule without reinstalling).
const GitattributesPath = ".gitattributes"

// gitattributesHeader introduces the block MergeGitattributes adds to a project's own file. The
// plugin's scaffold.sh writes the same two lines (TestScaffoldMergesGitattributes holds them equal).
const gitattributesHeader = "# Clauductor: LF line endings on every OS, so shell scripts run under WSL2 and Git for Windows\n" +
	"# (.claude/checks/line-endings.sh holds it). Your own lines below still override these.\n"

// MergeGitattributes returns the project's .gitattributes (have) with the template's rules (want)
// that it lacks, and the rules added. The missing rules are PREPENDED, never appended: when several
// lines match a path, the LAST one wins for each attribute, so the project's own lines (a
// `*.bat text eol=crlf`, say) must stay after the template's catch-all `* text=auto eol=lf` to keep
// overriding it. An empty project file becomes the template's file whole.
func MergeGitattributes(have, want string) (string, []string) {
	var missing []string
	for _, l := range GitignoreEntries(want) {
		if !hasLine(have, l) {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		return have, nil
	}
	if strings.TrimSpace(have) == "" {
		return want, missing
	}
	return gitattributesHeader + strings.Join(missing, "\n") + "\n\n" + have, missing
}
