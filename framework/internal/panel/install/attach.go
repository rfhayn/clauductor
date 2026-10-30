package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/signals"
)

// `stridal panel attach` (PANEL-42) serves a repository without writing to it: it
// reads the repository as `panel init` does, writes the result outside it, to
// ~/.stridal/projects/<id>/panel.json (the sidecar), and registers the repository
// with that file as its config, untrusted. design.md D2 to D4.

// AttachOptions attaches one repository.
type AttachOptions struct {
	Home    string
	Project string // any path inside the repository's main worktree
	ID      string // "" for the slug of init's name for it
	Run     signals.Runner
	Now     time.Time
	// MakeDefault makes the project the default, as `panel add --default` does.
	MakeDefault bool
}

// AttachResult is what Attach wrote and registered.
type AttachResult struct {
	Init InitResult // the sidecar's path and bytes, and init's notes
	Add  AddResult
	// InPlace: the repository was registered already with no config that loads, and
	// keeps its id, socket and lane registry (design.md D4).
	InPlace bool
}

// Attach writes a repository's sidecar and registers the repository with it. It
// never overwrites a sidecar (D3), and a refused registration removes what it wrote,
// so a refused attach leaves no file and no entry.
func Attach(ctx context.Context, o AttachOptions) (AttachResult, error) {
	if o.Run == nil {
		o.Run = signals.ExecRunner
	}
	// The repository as `panel add` resolves it: its main worktree, a linked one
	// refused.
	root, err := mainRoot(ctx, o.Run, o.Project)
	if err != nil {
		return AttachResult{}, err
	}
	reg, err := config.LoadProjects(o.Home)
	if err != nil {
		return AttachResult{}, err
	}
	id, inPlace := o.ID, false
	if e := reg.Find(root); e != nil {
		// D4: only a project that cannot load is attached in place (Standing Tee's
		// state); a working one keeps the config it reads.
		_, ownErr := os.Lstat(config.OwnConfigPath(root))
		if e.Config != "" || !errors.Is(ownErr, os.ErrNotExist) {
			// The command lines are shell-quoted, so they run as printed (PANEL-47).
			return AttachResult{}, fmt.Errorf("%s is registered already as %q, reading %s; `stridal panel remove %s` first, or `stridal panel add --project %s --config <file>` to switch its config on purpose",
				root, e.ID, e.ConfigPath(), ShellWord(e.ID), ShellWord(root))
		}
		if id != "" && id != e.ID {
			return AttachResult{}, fmt.Errorf("%s is registered already as %q, and attaching it in place keeps that id; drop --id", root, e.ID)
		}
		id, inPlace = e.ID, true
	} else {
		if common, err := commonDir(ctx, o.Run, root); err == nil {
			if other := registeredAs(ctx, o.Run, reg, root, common); other != "" {
				return AttachResult{}, fmt.Errorf("%s is the repository registered already as %q", root, other)
			}
		}
		if id == "" {
			id = reg.Slug(InitName(root))
		}
		// The id is a path segment of the sidecar: check it before it names one.
		if !config.ProjectIDRe.MatchString(id) {
			return AttachResult{}, fmt.Errorf("project id %q must match %s", id, config.ProjectIDRe)
		}
		if reg.Find(id) != nil {
			return AttachResult{}, fmt.Errorf("project id %q is taken; pass --id", id)
		}
	}

	path := config.SidecarPath(signals.ResolvePath(o.Home), id)
	// ATTACH-2: a home inside the repository (dotfiles kept in a work tree at ~, or a
	// bare attach from a folder under ~ in no other repository) would put the sidecar,
	// and the registry beside it, under the root. Refused before anything is written:
	// the file would be an in-repository config, which D5 judges differently.
	for _, p := range []string{path, config.ProjectsPath(o.Home)} {
		if r := config.ResolveExisting(p); config.Within(r, signals.ResolvePath(root)) {
			return AttachResult{}, fmt.Errorf("%s is inside the repository %s, and attach writes nothing in a repository; attach with HOME outside it",
				r, signals.ResolvePath(root))
		}
	}
	exists := func() error {
		serve := fmt.Sprintf("`stridal panel add --project %s --config %s` serves it as it is", ShellWord(root), ShellWord(path))
		if inPlace { // D4 keeps the id, so another one is no way out
			return fmt.Errorf("%s exists already, and attach never overwrites it: %s", path, serve)
		}
		return fmt.Errorf("%s exists already, and attach never overwrites it: %s, or `--id <other>` writes a new one", path, serve)
	}
	plan, err := PlanInit(ctx, o.Run, root, InitTarget{Path: path, Sidecar: true})
	if errors.Is(err, ErrConfigExists) {
		return AttachResult{}, exists()
	}
	if err != nil {
		return AttachResult{}, err
	}
	// What this attach creates, so a refusal removes exactly that.
	var made []string
	for d := filepath.Dir(path); ; d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err == nil || d == filepath.Dir(d) {
			break
		}
		made = append(made, d)
	}
	removeMade := func() {
		for _, d := range made { // deepest first; each is empty once its child is gone
			os.Remove(d)
		}
	}
	if err := WriteInit(plan); err != nil {
		// WriteInit removes a file it made and could not finish; the folders are
		// this attach's. A file that was there (ErrConfigExists) keeps its folder,
		// since os.Remove takes only an empty one.
		removeMade()
		if errors.Is(err, ErrConfigExists) {
			return AttachResult{}, exists()
		}
		return AttachResult{}, err
	}
	add, err := AddProject(ctx, AddOptions{Home: o.Home, Project: root, Config: path, ID: id, Run: o.Run, Now: o.Now,
		Strict: true, MakeDefault: o.MakeDefault})
	if err != nil {
		os.Remove(path)
		removeMade()
		return AttachResult{}, err
	}
	return AttachResult{Init: plan, Add: add, InPlace: inPlace}, nil
}
