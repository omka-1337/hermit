package launch

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"hermit/internal/library"
	"hermit/internal/modinstall"
	"hermit/internal/platform"
)

// Wrapper runs a game command (Steam's %command%) with the active profile of
// the game linked in.
type Wrapper struct {
	Lib       *library.Library
	Installer *modinstall.Installer
	// Notify shows a desktop notification; failures to set up mods are
	// reported through it because Steam hides the wrapper's output.
	Notify func(summary, body string)
}

var appIDArg = regexp.MustCompile(`^AppId=(\d+)$`)

// Run parses "[--game <id>] -- <command...>", runs the command and returns
// its exit code. Mod setup problems never prevent the game from starting: the
// game is then launched without mods.
func (w *Wrapper) Run(args []string) int {
	gameID, command, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	logOut := io.Writer(os.Stderr)
	// Inside the AppImage, give the game the environment Steam provided, not
	// the one AppRun prepared for Hermit's bundled GTK and WebKit.
	plan, setupErr := w.Prepare(gameID, command, platform.OriginalEnv(), os.Getpid(), &logOut)
	logger := log.New(logOut, "[hermit] ", log.LstdFlags)
	switch {
	case setupErr != nil:
		logger.Printf("launching without mods: %v", setupErr)
		if w.Notify != nil {
			w.Notify("Mods were not loaded", setupErr.Error())
		}
	case !plan.Modded:
		logger.Printf("profile has no active BepInEx, launching without mods")
	default:
		logger.Printf("launching with profile %s: %q", plan.ProfileID, plan.Command)
	}

	code := runCommand(plan.Command, plan.Env, logger)
	if plan.GameID != "" {
		if err := w.Cleanup(plan.GameID); err != nil {
			logger.Printf("cleanup: %v", err)
		}
	}
	return code
}

func parseArgs(args []string) (gameID string, command []string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--game":
			if i+1 >= len(args) {
				return "", nil, errors.New("--game needs a value")
			}
			gameID = args[i+1]
			i++
		case "--":
			command = args[i+1:]
			if len(command) == 0 {
				return "", nil, errors.New("no command after --")
			}
			return gameID, command, nil
		default:
			return "", nil, fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	return "", nil, errors.New(`usage: run [--game <id>] -- <command...> (in Steam: run -- %command%)`)
}

// Plan is how a game should be started. Prepare fills it in fully only when
// mods can be loaded; on error the original command and environment are
// returned unchanged, so the game still starts.
type Plan struct {
	Command []string
	Env     []string
	// Modded is false when the profile has no active BepInEx.
	Modded bool
	// GameID and ProfileID are empty when the game could not be resolved;
	// GameID is what Cleanup needs to finish the session.
	GameID    string
	ProfileID string
}

// nativeLaunchers are scripts that BepInEx packs for native Linux games ship in
// the profile root, in the order r2modman tries them. They take Steam's
// %command%, find the game executable in it and load BepInEx from their own
// directory, so nothing needs to be linked into the game folder.
var nativeLaunchers = []string{"run_bepinex.sh", "start_game_bepinex.sh"}

// Prepare links the active profile of a game and returns how to launch it.
// The session is owned by pid: it is the process expected to run the game, so
// that a leftover session can be told from a running one. Whoever calls
// Prepare must call Cleanup once the game has exited.
func (w *Wrapper) Prepare(gameID string, command, env []string, pid int, logOut *io.Writer) (Plan, error) {
	plan := Plan{Command: command, Env: env}
	if w.Lib == nil || w.Installer == nil {
		return plan, errors.New("mod manager data could not be opened")
	}
	game, err := w.findGame(gameID, plan.Command)
	if err != nil {
		return plan, err
	}
	plan.GameID = game.ID
	dataDir, err := w.Lib.GameDataDir(game.ID)
	if err != nil {
		return plan, err
	}
	if logOut != nil {
		if f, err := os.Create(filepath.Join(dataDir, "launch.log")); err == nil {
			*logOut = io.MultiWriter(os.Stderr, f)
		}
	}
	if game.Runtime != library.RuntimeProton && game.Runtime != library.RuntimeNative {
		return plan, fmt.Errorf("%s: could not tell whether the game runs natively or through Proton", game.Name)
	}

	prev, err := ReadSession(dataDir)
	if err != nil {
		return plan, err
	}
	if prev.Running() {
		return plan, fmt.Errorf("%s is already running (pid %d)", game.Name, prev.PID)
	}
	if prev != nil {
		// Left over from a session that did not exit cleanly.
		Unlink(game.Path, prev.Links, w.Lib.Root())
	}

	profile, err := w.Installer.Refresh(game.ID, game.ActiveProfile)
	if err != nil {
		return plan, err
	}
	profileDir, err := w.Lib.ProfileDir(game.ID, profile.ID)
	if err != nil {
		return plan, err
	}

	session := Session{ProfileID: profile.ID, PID: pid, StartedAt: time.Now(), Links: []string{}}
	modded := false
	if game.Runtime == library.RuntimeNative {
		launcher, err := nativeLauncher(profileDir)
		if err != nil {
			return plan, err
		}
		if launcher != "" {
			command = append([]string{launcher}, command...)
			modded = true
		}
	} else {
		session.Links, err = LinkProfile(game.Path, profileDir, w.Lib.Root())
		if err != nil {
			return plan, err
		}
		if len(session.Links) > 0 {
			env = withDLLOverride(env, "winhttp", "n,b")
			modded = true
		}
	}
	session.Modded = modded
	if err := writeSession(dataDir, session); err != nil {
		Unlink(game.Path, session.Links, w.Lib.Root())
		return plan, err
	}

	plan.Command, plan.Env, plan.Modded, plan.ProfileID = command, env, modded, profile.ID
	return plan, nil
}

// Cleanup finishes the session Prepare started: it writes the report of the
// game session and removes the links from the game folder. It is safe to call
// when there is no session, and from a different process than Prepare.
func (w *Wrapper) Cleanup(gameID string) error {
	if w.Lib == nil {
		return errors.New("mod manager data could not be opened")
	}
	game, err := w.Lib.GetGame(gameID)
	if err != nil {
		return err
	}
	dataDir, err := w.Lib.GameDataDir(game.ID)
	if err != nil {
		return err
	}
	session, err := ReadSession(dataDir)
	if err != nil || session == nil {
		return err
	}

	var reportErr error
	if session.Modded {
		profileDir, err := w.Lib.ProfileDir(game.ID, session.ProfileID)
		if err != nil {
			return err
		}
		profile, err := w.Lib.GetProfile(game.ID, session.ProfileID)
		if err != nil {
			return err
		}
		reportErr = SaveReport(dataDir, BuildReport(profileDir, *session, profile.Mods))
	}
	return errors.Join(reportErr, Unlink(game.Path, session.Links, w.Lib.Root()), os.Remove(sessionPath(dataDir)))
}

// nativeLauncher returns the BepInEx launcher script of a profile, made
// executable, or "" when the profile has no active loader.
func nativeLauncher(profileDir string) (string, error) {
	if _, err := os.Stat(filepath.Join(profileDir, "BepInEx", "core")); err != nil {
		return "", nil
	}
	for _, name := range nativeLaunchers {
		script := filepath.Join(profileDir, name)
		fi, err := os.Stat(script)
		if err != nil {
			continue
		}
		// Archives usually lose the executable bit.
		if fi.Mode()&0o111 != 0o111 {
			if err := os.Chmod(script, fi.Mode()|0o755); err != nil {
				return "", err
			}
		}
		return script, nil
	}
	return "", errors.New("the installed BepInEx pack has no Linux launcher (run_bepinex.sh or start_game_bepinex.sh); this game may only support BepInEx through Proton")
}

// findGame resolves the game from --game, Steam's SteamAppId variable, or the
// AppId=<n> argument Steam passes to its reaper process.
func (w *Wrapper) findGame(gameID string, command []string) (library.Game, error) {
	if gameID != "" {
		return w.Lib.GetGame(gameID)
	}
	appID := os.Getenv("SteamAppId")
	if appID == "" {
		for _, arg := range command {
			if m := appIDArg.FindStringSubmatch(arg); m != nil {
				appID = m[1]
				break
			}
		}
	}
	if appID == "" {
		return library.Game{}, errors.New("cannot tell which game is launched; pass --game <id>")
	}
	return w.Lib.FindGameBySteamAppID(appID)
}

// withDLLOverride adds a Wine DLL override unless the DLL is already configured.
func withDLLOverride(env []string, dll, mode string) []string {
	const key = "WINEDLLOVERRIDES="
	for i, kv := range env {
		value, ok := strings.CutPrefix(kv, key)
		if !ok {
			continue
		}
		for _, entry := range strings.Split(value, ";") {
			names, _, _ := strings.Cut(entry, "=")
			for _, name := range strings.Split(names, ",") {
				if strings.EqualFold(strings.TrimSpace(name), dll) {
					return env
				}
			}
		}
		if value != "" {
			value += ";"
		}
		out := append([]string(nil), env...)
		out[i] = key + value + dll + "=" + mode
		return out
	}
	return append(env, key+dll+"="+mode)
}

func runCommand(command, env []string, logger *log.Logger) int {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	cmd.Dir = platform.OriginalWorkingDir()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		logger.Printf("start %s: %v", command[0], err)
		return 127
	}

	// Forward termination signals so stopping the game from Steam works, while
	// the wrapper stays alive to clean up.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigs:
				_ = cmd.Process.Signal(s)
			case <-done:
				return
			}
		}
	}()

	err := cmd.Wait()
	signal.Stop(sigs)
	close(done)
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exitErr):
		return exitErr.ExitCode()
	default:
		logger.Printf("wait: %v", err)
		return 1
	}
}
