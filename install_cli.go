package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"hermit/internal/app"
	"hermit/internal/fomod"
	"hermit/internal/library"
	"hermit/internal/settings"
)

const installUsage = `usage: hermit install [--game <id>] [--profile <id>] [--choose "[Step/]Group=Option[,Option]"]... [--defaults] <archive>...
  Installs mod archives (zip, 7z, rar) into a profile of a Bethesda game, the active one unless --profile says
  otherwise. A FOMOD installer takes its options from --choose, or its defaults with --defaults; with neither, it
  asks for them step by step when run in a terminal.`

type installArgs struct {
	game, profile string
	choose        []string
	defaults      bool
	files         []string
}

func parseInstallArgs(args []string) (installArgs, error) {
	var a installArgs
	for i := 0; i < len(args); i++ {
		value := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", args[i])
			}
			i++
			return args[i], nil
		}
		var err error
		switch args[i] {
		case "--game":
			a.game, err = value()
		case "--profile":
			a.profile, err = value()
		case "--choose":
			var c string
			c, err = value()
			a.choose = append(a.choose, c)
		case "--defaults":
			a.defaults = true
		default:
			if strings.HasPrefix(args[i], "--") {
				return a, fmt.Errorf("unexpected option %q", args[i])
			}
			a.files = append(a.files, args[i])
		}
		if err != nil {
			return a, err
		}
	}
	if len(a.files) == 0 {
		return a, errors.New("no archive given")
	}
	return a, nil
}

// installCommand installs mod archives from the command line.
func installCommand(args []string) int {
	if slices.Contains(args, "-h") || slices.Contains(args, "--help") {
		fmt.Println(installUsage)
		return 0
	}
	a, err := parseInstallArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n%s\n", err, installUsage)
		return 2
	}
	b, err := newBackend()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	game, err := installTarget(b.lib, settings.NewStore(b.root), a.game)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	profile := a.profile
	if profile == "" {
		profile = game.ActiveProfile
	}
	svc := app.NewBethesdaService(b.lib, b.steamRoots)
	svc.OnUnpack = func(p app.UnpackProgress) {
		if p.Total > 0 {
			fmt.Fprintf(os.Stderr, "\runpacking %s: %d%%", p.File, p.Done*100/p.Total)
		}
	}
	interactive := !a.defaults && len(a.choose) == 0 && isTerminal(os.Stdin)
	for _, file := range a.files {
		if err := installOne(svc, game, profile, file, a, interactive); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
			return 1
		}
	}
	return 0
}

func installOne(svc *app.BethesdaService, game library.Game, profile, file string, a installArgs, interactive bool) error {
	prepared, err := svc.PrepareInstall(game.ID, profile, file)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	var choices fomod.Choices
	if prepared.Fomod {
		if choices, err = pickOptions(svc, prepared, a.choose, interactive, os.Stdin, os.Stdout); err != nil {
			svc.CancelInstall(prepared.Session)
			return err
		}
	}
	p, err := svc.FinishInstall(prepared.Session, choices)
	if err != nil {
		svc.CancelInstall(prepared.Session)
		return err
	}
	m := p.Mods[len(p.Mods)-1]
	fmt.Printf("installed %s %s into %s of %s\n", m.Name, m.Version, p.Name, game.Name)
	if m.Notes != "" {
		fmt.Println("  " + m.Notes)
	}
	return nil
}

// pickOptions settles a FOMOD's options: from --choose, from the user when
// interactive, or its defaults.
func pickOptions(svc *app.BethesdaService, prepared app.PreparedMod, choose []string, interactive bool, in io.Reader, out io.Writer) (fomod.Choices, error) {
	view, err := svc.FomodView(prepared.Session, nil)
	if err != nil {
		return nil, err
	}
	choices := view.Choices
	if len(choose) > 0 {
		// Options picked can bring steps up, so names are matched again
		// until nothing changes.
		for range 10 {
			before := fmt.Sprint(choices)
			for _, c := range choose {
				if err := applyChoice(view, choices, c); err != nil && !errors.Is(err, errNoGroup) {
					return nil, err
				}
			}
			if view, err = svc.FomodView(prepared.Session, choices); err != nil {
				return nil, err
			}
			choices = view.Choices
			if fmt.Sprint(choices) == before {
				break
			}
		}
		for _, c := range choose {
			if err := applyChoice(view, choices, c); err != nil {
				return nil, err
			}
		}
	} else if interactive {
		reader := bufio.NewReader(in)
		fmt.Fprintf(out, "%s has an installer.\n", prepared.Name)
		for shown := 0; shown < len(view.Steps); shown++ {
			step := view.Steps[shown]
			fmt.Fprintf(out, "\n== %s\n", step.Name)
			for g, group := range step.Groups {
				picked, err := askGroup(reader, out, group)
				if err != nil {
					return nil, err
				}
				if picked != nil {
					choices[step.Index][g] = picked
				}
			}
			if view, err = svc.FomodView(prepared.Session, choices); err != nil {
				return nil, err
			}
			choices = view.Choices
		}
	}
	if !view.Ready {
		for _, step := range view.Steps {
			for _, group := range step.Groups {
				if group.Problem != "" {
					return nil, fmt.Errorf("%s / %s: %s", step.Name, group.Name, strings.ToLower(group.Problem))
				}
			}
		}
	}
	return choices, nil
}

var errNoGroup = errors.New("no such group")

// applyChoice picks the options "[Step/]Group=Option[,Option]" names among
// the steps that show.
func applyChoice(view fomod.View, choices fomod.Choices, choice string) error {
	where, options, ok := strings.Cut(choice, "=")
	if !ok {
		return fmt.Errorf("%q is not [Step/]Group=Option", choice)
	}
	stepName, groupName, hasStep := strings.Cut(where, "/")
	if !hasStep {
		stepName, groupName = "", where
	}
	for _, step := range view.Steps {
		if hasStep && !strings.EqualFold(strings.TrimSpace(step.Name), strings.TrimSpace(stepName)) {
			continue
		}
		for g, group := range step.Groups {
			if !strings.EqualFold(strings.TrimSpace(group.Name), strings.TrimSpace(groupName)) {
				continue
			}
			var picked []int
			for _, name := range strings.Split(options, ",") {
				i := slices.IndexFunc(group.Plugins, func(p fomod.PluginView) bool {
					return strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(name))
				})
				if i < 0 {
					return fmt.Errorf("%s has no option %q", group.Name, strings.TrimSpace(name))
				}
				picked = append(picked, i)
			}
			choices[step.Index][g] = picked
			return nil
		}
	}
	return fmt.Errorf("%q: %w", where, errNoGroup)
}

// askGroup asks which options of a group to install; nil keeps what is
// marked.
func askGroup(r *bufio.Reader, out io.Writer, group fomod.GroupView) ([]int, error) {
	fmt.Fprintf(out, "\n%s (%s)\n", group.Name, groupHint(group.Type))
	for i, p := range group.Plugins {
		mark := " "
		if p.Selected {
			mark = "x"
		}
		note := ""
		if p.Type != fomod.Optional {
			note = " (" + string(p.Type) + ")"
		}
		fmt.Fprintf(out, "  %2d) [%s] %s%s\n", i+1, mark, p.Name, note)
		if d := strings.SplitN(p.Description, "\n", 2)[0]; d != "" {
			fmt.Fprintf(out, "        %s\n", d)
		}
	}
	if group.Type == fomod.SelectAll {
		return nil, nil
	}
	for {
		fmt.Fprint(out, "pick (numbers, comma-separated; Enter keeps the marked ones): ")
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return nil, nil
		}
		var picked []int
		bad := false
		for _, part := range strings.Split(line, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 1 || n > len(group.Plugins) || group.Plugins[n-1].Type == fomod.NotUsable {
				bad = true
				break
			}
			picked = append(picked, n-1)
		}
		if one := group.Type == fomod.SelectExactlyOne || group.Type == fomod.SelectAtMostOne; !bad && (!one || len(picked) <= 1) {
			return picked, nil
		}
		fmt.Fprintln(out, "that does not fit this group")
	}
}

func groupHint(t fomod.GroupType) string {
	switch t {
	case fomod.SelectExactlyOne:
		return "pick one"
	case fomod.SelectAtMostOne:
		return "pick one or none"
	case fomod.SelectAtLeastOne:
		return "pick at least one"
	case fomod.SelectAll:
		return "all are installed"
	}
	return "pick any"
}

// installTarget is the game to install into: the one named, the only
// Bethesda game added, or the one open last.
func installTarget(lib *library.Library, store *settings.Store, id string) (library.Game, error) {
	if id != "" {
		return lib.GetGame(id)
	}
	games, err := lib.ListGames()
	if err != nil {
		return library.Game{}, err
	}
	var creation []library.Game
	for _, g := range games {
		if g.Engine == library.EngineCreation {
			creation = append(creation, g)
		}
	}
	if len(creation) == 1 {
		return creation[0], nil
	}
	if st, err := store.Get(); err == nil {
		if i := slices.IndexFunc(creation, func(g library.Game) bool { return g.ID == st.LastGame }); i >= 0 {
			return creation[i], nil
		}
	}
	var ids []string
	for _, g := range creation {
		ids = append(ids, g.ID)
	}
	if len(ids) == 0 {
		return library.Game{}, errors.New("no Bethesda game is added to Hermit")
	}
	return library.Game{}, fmt.Errorf("which game? pass --game with one of: %s", strings.Join(ids, ", "))
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
