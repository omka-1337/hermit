package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"hermit/internal/launch"
)

// The command line exists for the launch wrapper. "run" starts a game itself;
// "prepare" and "cleanup" do the same work in two steps, for a caller that has
// to start the game on its own — a Flatpak build cannot start it from inside
// the sandbox, so a small script on the host does that part.
const cliUsage = `usage:
  hermit run     [--game <id>] -- <command...>   start a game with its active profile
  hermit prepare [--game <id>] [--appid <n>] [--pid <n>] -- <command...>
                                                 link the profile, print shell code to apply
  hermit cleanup --game <id>                     end the session: write the report, remove links
  hermit --setup                                 open the first-run window again`

// runCLI handles a command line invocation and reports whether it did.
func runCLI(args []string) (code int, handled bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "run":
		return runWrapper(args[1:]), true
	case "prepare":
		return prepareCommand(args[1:]), true
	case "cleanup":
		return cleanupCommand(args[1:]), true
	case "-h", "--help", "help":
		fmt.Println(cliUsage)
		return 0, true
	}
	return 0, false
}

// prepareCommand links the active profile and prints shell code that applies
// the result: the environment the game needs, any launcher to put in front of
// the command, and the game id for the matching cleanup. Setting up mods must
// never stop a game from starting, so problems are reported on stderr and the
// caller still runs the command it had.
func prepareCommand(args []string) int {
	gameID, appID, pid, command, err := parsePrepareArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	// The wrapper finds the game by this variable when no id is given; in a
	// sandbox the caller has to pass it along.
	if appID != "" {
		os.Setenv("SteamAppId", appID)
	}

	w := &launch.Wrapper{}
	if b, err := newBackend(); err == nil {
		w.Lib, w.Installer = b.lib, b.installer
	}
	plan, err := w.Prepare(gameID, command, os.Environ(), pid, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hermit: launching without mods: %v\n", err)
		return 1
	}

	var out strings.Builder
	for _, assignment := range changedEnv(os.Environ(), plan.Env) {
		name, value, _ := strings.Cut(assignment, "=")
		fmt.Fprintf(&out, "export %s=%s\n", name, shellQuote(value))
	}
	// Native BepInEx packs are launched through their own script, which takes
	// the game command as its arguments.
	if extra := len(plan.Command) - len(command); extra > 0 {
		quoted := make([]string, extra)
		for i, arg := range plan.Command[:extra] {
			quoted[i] = shellQuote(arg)
		}
		fmt.Fprintf(&out, "set -- %s \"$@\"\n", strings.Join(quoted, " "))
	}
	fmt.Fprintf(&out, "HERMIT_GAME=%s\n", shellQuote(plan.GameID))
	fmt.Print(out.String())
	return 0
}

// cleanupCommand ends the session started by prepare.
func cleanupCommand(args []string) int {
	var gameID string
	for i := 0; i < len(args); i++ {
		if args[i] == "--game" && i+1 < len(args) {
			gameID = args[i+1]
			i++
			continue
		}
		fmt.Fprintln(os.Stderr, "usage: hermit cleanup --game <id>")
		return 2
	}
	if gameID == "" {
		fmt.Fprintln(os.Stderr, "usage: hermit cleanup --game <id>")
		return 2
	}

	w := &launch.Wrapper{}
	if b, err := newBackend(); err == nil {
		w.Lib, w.Installer = b.lib, b.installer
	}
	if err := w.Cleanup(gameID); err != nil {
		fmt.Fprintf(os.Stderr, "hermit: cleanup: %v\n", err)
		return 1
	}
	return 0
}

func parsePrepareArgs(args []string) (gameID, appID string, pid int, command []string, err error) {
	pid = os.Getpid()
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--game", "--appid", "--pid":
			if i+1 >= len(args) {
				return "", "", 0, nil, fmt.Errorf("%s needs a value", args[i])
			}
			value := args[i+1]
			i++
			switch args[i-1] {
			case "--game":
				gameID = value
			case "--appid":
				appID = value
			case "--pid":
				// An empty or unparsable pid keeps this process as the owner.
				if n, convErr := strconv.Atoi(value); convErr == nil && n > 0 {
					pid = n
				}
			}
		case "--":
			command = args[i+1:]
			if len(command) == 0 {
				return "", "", 0, nil, errors.New("no command after --")
			}
			return gameID, appID, pid, command, nil
		default:
			return "", "", 0, nil, fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	return "", "", 0, nil, errors.New(cliUsage)
}

// changedEnv returns the entries of want that the current environment does not
// already have.
func changedEnv(current, want []string) []string {
	have := make(map[string]string, len(current))
	for _, entry := range current {
		name, value, _ := strings.Cut(entry, "=")
		have[name] = value
	}
	var changed []string
	for _, entry := range want {
		name, value, _ := strings.Cut(entry, "=")
		if old, ok := have[name]; !ok || old != value {
			changed = append(changed, entry)
		}
	}
	return changed
}

// shellQuote wraps a value in single quotes so any shell reads it verbatim.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
