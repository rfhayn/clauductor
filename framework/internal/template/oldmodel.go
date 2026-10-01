package template

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Clauductor's first operating model (lock-based skills, session hooks that called the binary)
// was installed by the same `clauductor install`. A repository that runs it is clauductor's to
// update, but install used to recognise it only by orchestration/config.json, runtime state that
// is gitignored, so a fresh clone of such a repository looked foreign and was refused (OPS-8
// rehearsal, finding 1). And once the current model was installed over it, nothing named the old
// model's files the new one replaced: 16 skills, 2 hooks and their settings registrations stayed
// behind (findings 2 and 7).
//
// This file is what is known about the old model, from tracked files only: its framework paths,
// and the sha256 of every version of them that the template (before OPS-1, 5a04478) and this
// repository's own copy (before OPS-8, 649aa49) ever held, taken from the git history of
// template/.claude and .claude ({skills,hooks,agents,statusline.sh}). It never changes again: the
// old model is no longer developed.

// oldModelSkills are the old model's skills that the current model does not ship. In the old
// model a skill was framework tier (update overwrote it), so a project's copy is the model's
// whether or not it was edited.
var oldModelSkills = []string{
	"assign", "blocked", "build", "claim", "commit", "done", "handoff", "milestone-complete",
	"new-milestone", "pane", "pr", "prd-audit", "release", "review", "skills", "spawn",
	"start-work", "status", "supervisor",
}

// oldModelDistinctive are the old skills whose names a project of its own is unlikely to use:
// three of them together are a sign of the old model even when every one was edited.
var oldModelDistinctive = []string{
	"assign", "blocked", "claim", "handoff", "milestone-complete", "new-milestone", "pane",
	"spawn", "start-work", "supervisor",
}

// oldModelHooks are the old model's hook scripts (each called the clauductor binary).
var oldModelHooks = []string{
	"auto-lock.sh", "doc-freshness.sh", "heartbeat.sh", "lock-guard.sh", "session-register.sh",
	"status-sync.sh",
}

// oldModelAgents were doc tier in the old model too: removed only when unedited.
var oldModelAgents = []string{"pre-implementation.md", "session-wrap.md"}

// OldModelHook says whether a hook script name is one of the old model's.
func OldModelHook(script string) bool { return contains(oldModelHooks, script) }

// oldModelHashes is every version of the old model's files, by project path.
var oldModelHashes = map[string][]string{
	".claude/agents/pre-implementation.md": {
		"8793bb8eedfe03ce7d0a6cc9c5b50ca9516c5b0da16ff819221e516920dc09ec",
	},
	".claude/agents/session-wrap.md": {
		"b5f7752f00383df733212501e8510fad1c385d1666f931139b66c3828c40f983",
	},
	".claude/hooks/README.md": {
		"15213acfb31aaad0cba3aae793a2f87bb8ed5a5f97d3816a6b8bd9bd7a14756c",
		"e8dab68ce1d1dc23e994851d7fd1510a173e7e47d5af786e3ddead66652d6370",
	},
	".claude/hooks/auto-lock.sh": {
		"aff46373a7ef610bbb8401b601eb1b397b814b3fc9e619dc2371728a22d88065",
	},
	".claude/hooks/doc-freshness.sh": {
		"34260dac5dbd529db9455e229a37ec2958d033bd07d877f62e43852b6eeabfb5",
		"7037fa61ffc423c2a3cef93a28d913e9f637f4b3914bbbd28bf540dcbac0efef",
	},
	".claude/hooks/heartbeat.sh": {
		"29996db64fcad93dfb207afcd7d422cd1d9ee6181a053c34ed3c34aaf7e01d62",
	},
	".claude/hooks/lock-guard.sh": {
		"2096ae8dc47d4360050080548e94de033f2b867bd5030de801ca5c5bdb688c46",
		"c1813c962cd2de2e267191f4d3f3680332256d421978105bd55e79512457392a",
	},
	".claude/hooks/session-register.sh": {
		"bb7b7a66a7fbd5f33421ac9b26d3e5bf90d8423b4d50b6a496021cd283a20920",
	},
	".claude/hooks/status-sync.sh": {
		"9d8de0c6b355078e4ac6182b4be5e723179f8567b1885cd34ca624984ee53f13",
	},
	".claude/skills/architecture-audit/SKILL.md": {
		"5e3400aea4400531c82dd83f969625dd9bad7065c825d30daccf379591374511",
		"ff898df1d57ece7c422eab8fb1eb83c184a295ced27483de4fe837558f5ea948",
	},
	".claude/skills/assign/SKILL.md": {
		"3f02173db2ec51bd3718619717f8b2a5053d3cdb970d33b28c7a20364c6d1e44",
	},
	".claude/skills/blocked/SKILL.md": {
		"945ce56825858fa8ce27371d0d401646ace794c9b1d2d779dee4fac2ac8e59dc",
	},
	".claude/skills/build/SKILL.md": {
		"4226fd346948f8a15eb36b89c5ed896d6ec8e06bc82baf9b0fa886da6ed55859",
		"de302e8665d7aeab561817fa6b5c97f8b7f1ea83dbf2b2047ae6846f0f150981",
	},
	".claude/skills/claim/SKILL.md": {
		"351dfd78ac3b2a96a093900415cf3731f5feb99365febb60557eff2f75b41668",
		"f1653963d70a01cb647ea14b4e225f32d5d0be1799577e1d9c6e7e5ca5bddebe",
	},
	".claude/skills/commit/SKILL.md": {
		"006f8e26151a4771bcc518c89663ba2a303a4272474836f6a790bdba222e3e75",
		"d54225fa96685eab1ee7aafe10e12fef420b3fa0904a8a232f3089561c6afbb3",
		"dfb37299e436ca6902ef9db2e9348f40fe4a6835685ac71f32d947217e32322f",
		"fa1d8b9278f1a2e26f5266a1479396c516dbfbd9ecfa8a960c31bdf3459081cc",
		"fa7d95baba1e8ef75ca52ea2ebb4a1a1f19a3110a251a25a90873c87ea090603",
	},
	".claude/skills/dev-journal/SKILL.md": {
		"86022f84d098b5f0f06d1df37d4b942323646ad17af22563d778575703551ee9",
		"e801345cb2ce6d7f0656e913289a36d913ca81f392bdc829de2ca9abe3de0499",
	},
	".claude/skills/done/SKILL.md": {
		"48ebee00bc214b814101d4dc4cdf1cbd57c77419fd00b7c73e2104527f3af78c",
	},
	".claude/skills/handoff/SKILL.md": {
		"213ca1fd23512401461850d54be26207cae3a050ef7190e2d0f7dc6953ae1593",
	},
	".claude/skills/log-insight/SKILL.md": {
		"84c1b1e4f99f17ac669e530cffb87f027255d469c5b67beb7a4349e3a879810d",
		"a1cde2adc70e864e52a8ac7582e53d4f73c3e36cd33d1caa2fbc68a1bb6f1f91",
		"e175edc097dc372d894e6ae62f63d59e284ac20a3ed80d775487fa2bebae484a",
	},
	".claude/skills/milestone-complete/SKILL.md": {
		"2b99aace5755a467fd4bf402bc795c3823ebe5431c4cbfaea93e05aa36164dbf",
		"8b10eca439121af1c6e8e6d386e3d7e278e49def0b54d5e19fd1c5801036c29e",
		"9f1c1021e8e37a8f7ae943a2e3628a109b06b741319af0e86d8c50b4d591a1b1",
		"b0b3b3cd73eb8585a353e3affd52977d2d7248fa60b5ba6bffe121f4d3cf9e66",
		"b292b61c7d91e6e2e36508e031a3a4c7018ad3cd72934d7dc4d58eafff708391",
		"f285563324e72345acd6cec620fc3dc482d8e34372aa8ca93dc5c97b3afa9de9",
	},
	".claude/skills/new-milestone/SKILL.md": {
		"09039461489c26245eaa62bd81a695c55983990cce698c93f92a328a376ac0cd",
		"2c4b85adfb2069b4c0a3a3ea5a433d0d7599091d06d4368f70a000c0173707d8",
		"3af18e715e3879a3f414d0b3e474fbd97371b3987f844867948a4b4cf5d7ad9b",
		"bb676c7fad3899352ec96150ba300c6007e5eac046bf474015f85ce1898f2cf2",
		"c0ca3b5bd96ff87db5f60f759390813a28c141c08471496501329054d8bd1160",
	},
	".claude/skills/pane/SKILL.md": {
		"77915b11ca1892e1861c4132b2b149b43757b50a8eb41fe427cba2833f4e28e2",
	},
	".claude/skills/pr/SKILL.md": {
		"10b9eaef430a7390f7dc90c05d0a9e153b870eecfd57efc4fe648f7aac554d97",
		"d0aeae9c6787bc947ddf169cb3deb5afc3e3e791c08cf3292679b44f299e3d2e",
		"dec331a8b40de36d1b1fbbafbe0c0b2606c81059e95e0e865449a2aa74bb0922",
		"f1c8c519013f3259106045d987e50caa10419e18e1a62a9c0125eafba65413dd",
	},
	".claude/skills/prd-audit/SKILL.md": {
		"e9ded4713297a9a48d54b12e473141b53d9c73524b47631cfe203180b7ded716",
	},
	".claude/skills/release-prep/SKILL.md": {
		"520a3756aee24b9eac4fe2b6118e5a0bc298f9b89b1ada5791d0175bd73d18a3",
		"f4f3b84d2d52593dfd73da4507619f5dada3b5fe67d8fffcd1dda544a6170d39",
	},
	".claude/skills/release/SKILL.md": {
		"0390aee73fb434a275bc080690d73062ec5cd5c6b64404167dd2d78a79ecd517",
	},
	".claude/skills/review/SKILL.md": {
		"e17a4b8130c70b9dbec5ea0bab29b2b3e778bf6494259299741a26079f84e23a",
	},
	".claude/skills/session-start/SKILL.md": {
		"1719556d68937bbe821ac57b4d99de6e20276711a2d8223a7321e1caca814fba",
		"55f9757956d78b3b3a87ebe4e21afcdc20b6a738fb9b3e08cb19bfb9aed68db9",
		"6c4294406199eff74352ccdbb44cd354b802b5a3356f22fda43b32775353641b",
		"8d0073f23fc85fdca00cba606fca0fbebc229e0c4c4d3657ae5335799b99e58d",
		"a886f00fce1dcba4ae0e4490553ca7db35ffdb4767bf7e977d5974661e153b50",
		"e2eeada4afb9df8ea22a28fd5f9f2237fd8880d558d3f6f32300e11efab078b4",
	},
	".claude/skills/skills/SKILL.md": {
		"081781a91a9032e4341f45b2e3ab75e81c05588c1f2870acf4ec131725a8e0ac",
		"0ac6708232f30b51f5657161ca681561b693a13918b024ce831a47903b2bf230",
		"0f9df0a8478e90594d4f03245ad8cfcb326395e5262c0be8ba336837be72f1c5",
		"39341cd0225fc8e60eccfc1eef62e29dfefb8805f162662801c50ea7a7aa2fa2",
		"58751f23c47b71274947f2eac948eb995ab1708441d61100caba979110e8b4fb",
		"80572989a18d3466cfb0340334b595e92404fbe1835e88454c3f22a7a30bc6b3",
		"b9fd56c58b3b131e637191c7420ec3fd6d925eba9b44b777b1d47af20f54db26",
		"c5309c34feecb5d302a3b40a0d34ffadc1bb6c18df9a08d6c98da7f95af1bf2a",
	},
	".claude/skills/spawn/SKILL.md": {
		"1dfe3fff5f2bb89c6ccec8626278341bac59abcd13703451ab56f1eaf7f8bcaa",
		"3eb3a2a2d0776b6f322382513502fe35778699b4c299e131ebfb10396dad491d",
		"60a9f178a4df2ea97c8ae9f90d2f45382daf216ccf4a9da604f24ab1ebe64922",
		"cbdd6f253b13b2ef5ff25face9186312cf668548085f449056f987ba9e2fe6e1",
		"d5049ed502e5eaffbd8b7b6b29a413b19444e5514c5f33b37b4f7bdab68fb680",
		"fc4f59e07e97e3f14fc5a52faaf83301952d455a41cd8168d057c9da74c97311",
	},
	".claude/skills/start-project/SKILL.md": {
		"8a08437f8ab41bf26c389900d9495f64c3bbf763f2ac14d1f766ee7830be0d09",
	},
	".claude/skills/start-work/SKILL.md": {
		"38812f0c6225e882376e3a6c017bbabf06bd3d29cbe583d08851a3d8a20eee3e",
		"a09511f6f1b9e3ef01fad922fa9c20a95c7417b9dc38639a8c94425f923aeb45",
		"a890858cb6e0b48b0109c478aaf0ae3f0f5176a2a8aede84438fbf8e56fa520b",
		"d26a4fac61c348d8583e1557615c4c5e7a2273c7fb07b6e330230890c31e717d",
	},
	".claude/skills/status/SKILL.md": {
		"a63d71ccdc1ae45acb9d99e9f9a3c839060005bd21e3134761549301153a7752",
	},
	".claude/skills/supervisor/SKILL.md": {
		"3db432a7e8c725e022279b3c81a0b99f379dc9b4aa470d1d8ed1a10bd68e88e2",
		"552da812bbe19f836530df96b3c324ab6d4b55f32e982c35b1e45076bc515f3a",
		"a7d94522a27ccea6dad889d936e946990d6b563809c9e51e31e5ed599c1da146",
		"ed056e19222e6276d63e751b3027859c1fd221961f85aa530f9f86dde7a36e02",
	},
	".claude/statusline.sh": {
		"55220397a69b6b7c5aeb8e111fdae1f879b3da5469d2914f2e594669e3b9aa01",
		"df95ae94cc5506b50246730404262b4b9360b07d55cd3401dc21fd2f9906d6b2",
	},
}

func sha256File(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

// OldModelUnchanged says whether the file at rel in dir is, byte for byte, a version of that file
// the old model shipped.
func OldModelUnchanged(dir, rel string) bool {
	hs, ok := oldModelHashes[rel]
	if !ok {
		return false
	}
	h, err := sha256File(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil && contains(hs, h)
}

// OldModelSigns lists the evidence that dir runs clauductor's old model, read from files a fresh
// clone has (and the old runtime state, when present). Empty means none.
func OldModelSigns(dir string) []string {
	var signs []string
	has := func(rel string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		return err == nil
	}
	var unchanged []string
	for _, rel := range sortedHashPaths() {
		if has(rel) && OldModelUnchanged(dir, rel) {
			unchanged = append(unchanged, rel)
		}
	}
	if len(unchanged) > 0 {
		signs = append(signs, fmt.Sprintf("%d file(s) byte-identical to the old model's (%s)", len(unchanged), abbrev(unchanged)))
	}
	var named []string
	for _, s := range oldModelDistinctive {
		if has(".claude/skills/" + s + "/SKILL.md") {
			named = append(named, s)
		}
	}
	if len(named) >= 3 {
		signs = append(signs, fmt.Sprintf("%d of the old model's skills (%s)", len(named), strings.Join(named, ", ")))
	}
	var hooks []string
	for _, h := range oldModelHooks {
		b, err := os.ReadFile(filepath.Join(dir, ".claude", "hooks", h))
		if err == nil && strings.Contains(string(b), "clauductor ") {
			hooks = append(hooks, h)
		}
	}
	if len(hooks) > 0 {
		signs = append(signs, "the old model's hooks, calling the clauductor binary ("+strings.Join(hooks, ", ")+")")
	}
	if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(SettingsPath))); err == nil {
		var reg []string
		for _, m := range hookScriptRe.FindAllStringSubmatch(string(b), -1) {
			if OldModelHook(m[1]) && !contains(reg, m[1]) {
				reg = append(reg, m[1])
			}
		}
		if len(reg) > 0 {
			signs = append(signs, SettingsPath+" registers the old model's hooks ("+strings.Join(reg, ", ")+")")
		}
	}
	if has("orchestration/config.json") {
		signs = append(signs, "orchestration/config.json (the old model's runtime state)")
	}
	return signs
}

// OldFile is a file of the old model's that the current model replaced.
type OldFile struct {
	Path      string `json:"path"`
	Unchanged bool   `json:"unchanged"` // byte-identical to a version the old model shipped
}

// StaleOldModel lists the old model's files in dir that the current model replaced and does not
// ship (templateFiles), which install and update offer to remove: every old-only skill's SKILL.md
// and every old hook script, edited or not (both were the old model's framework tier), and an old
// agent only when unedited. Nothing else: a file the old model did not ship is the project's.
func StaleOldModel(dir string, templateFiles []string) []OldFile {
	ships := map[string]bool{}
	for _, f := range templateFiles {
		ships[f] = true
	}
	var out []OldFile
	add := func(rel string, needUnchanged bool) {
		if ships[rel] {
			return
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return
		}
		u := OldModelUnchanged(dir, rel)
		if needUnchanged && !u {
			return
		}
		out = append(out, OldFile{Path: rel, Unchanged: u})
	}
	for _, s := range oldModelSkills {
		add(".claude/skills/"+s+"/SKILL.md", false)
	}
	for _, h := range oldModelHooks {
		add(".claude/hooks/"+h, false)
	}
	for _, a := range oldModelAgents {
		add(".claude/agents/"+a, true)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// PruneOldModel removes the listed files, and a skill directory left empty by it. It removes
// nothing else.
func PruneOldModel(dir string, files []OldFile) ([]string, error) {
	var removed []string
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed = append(removed, f.Path)
		if strings.HasPrefix(f.Path, ".claude/skills/") {
			_ = os.Remove(filepath.Dir(p)) // succeeds only when the directory is empty
		}
	}
	return removed, nil
}

func sortedHashPaths() []string {
	var ps []string
	for p := range oldModelHashes {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

func abbrev(list []string) string {
	if len(list) <= 4 {
		return strings.Join(list, ", ")
	}
	return strings.Join(list[:4], ", ") + fmt.Sprintf(", and %d more", len(list)-4)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
