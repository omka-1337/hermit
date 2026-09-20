package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"hermit/internal/app"
	"hermit/internal/github"
	"hermit/internal/launch"
	"hermit/internal/library"
	"hermit/internal/modinstall"
	"hermit/internal/platform"
	"hermit/internal/profileshare"
	"hermit/internal/settings"
	"hermit/internal/steam"
	"hermit/internal/thunderstore"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[modinstall.Progress](app.InstallProgressEvent)
	application.RegisterEvent[profileshare.ImportProgress](app.ImportProgressEvent)
}

type backend struct {
	root       string
	steamRoots []string
	lib        *library.Library
	ts         *thunderstore.Client
	github     *github.Client
	installer  *modinstall.Installer
}

func newBackend() (*backend, error) {
	root := filepath.Join(xdg.DataHome, app.ID)
	migrateDataDir(filepath.Join(xdg.DataHome, "bepinexmodmanager"), root)
	steamRoots := steam.DefaultRoots()
	lib, err := library.New(root, steamRoots)
	if err != nil {
		return nil, err
	}
	ts := thunderstore.NewClient(
		thunderstore.DefaultBaseURL,
		app.ID+"/"+app.Version,
		filepath.Join(root, "cache", "thunderstore"),
	)
	rules := func(ctx context.Context, game library.Game) modinstall.Rules {
		schema, err := ts.Schema(ctx)
		if err != nil {
			log.Printf("install rules: %v; using defaults", err)
		}
		return modinstall.RulesFromSchema(schema, game.SteamAppID, game.Executable)
	}
	gh := github.NewClient(github.DefaultAPIURL, app.ID+"/"+app.Version, filepath.Join(root, "cache", "github"))
	installer := modinstall.NewInstaller(lib, ts, rules)
	installer.SetGitHub(gh)
	return &backend{
		root:       root,
		steamRoots: steamRoots,
		lib:        lib,
		ts:         ts,
		github:     gh,
		installer:  installer,
	}, nil
}

func main() {
	// "run -- <command>" is the Steam launch options wrapper; no GUI.
	if code, handled := runCLI(os.Args[1:]); handled {
		os.Exit(code)
	}

	platform.ConfigureRendering()
	platform.SetProgramName(app.ID, app.Name)

	b, err := newBackend()
	if err != nil {
		log.Fatal(err)
	}
	settingsStore := settings.NewStore(b.root)
	stored, err := settingsStore.Get()
	if err != nil {
		log.Printf("settings: %v", err)
	}

	wailsApp := application.New(application.Options{
		Name:        app.Name,
		Description: "BepInEx mod manager for Linux",
		Services: []application.Service{
			application.NewService(app.NewInfoService(b.github)),
			application.NewService(b.lib),
			application.NewService(settingsStore),
			application.NewService(app.NewBrowseService(b.lib, b.ts, settingsStore)),
			application.NewService(app.NewInstallService(b.installer, b.github)),
			application.NewService(app.NewLaunchService(b.lib, b.steamRoots)),
			application.NewService(app.NewIconService(b.steamRoots)),
			application.NewService(app.NewConfigService(b.lib)),
			application.NewService(app.NewShareService(
				profileshare.NewSharer(b.lib, b.installer, b.ts, filepath.Join(b.root, "cache", "imports")),
			)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	windowOptions := application.WebviewWindowOptions{
		Title: app.Name,
		// The frontend draws its own title bar and window controls.
		Frameless: true,
		// Shown by showFramelessWindow once the native window exists.
		Hidden:           true,
		Width:            1200,
		Height:           760,
		MinWidth:         900,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(9, 9, 11), // same as the page background
		URL:              "/",
	}
	if deckLayout(stored.UIMode) {
		// The Steam Deck screen is 1280x800; fill it and allow smaller windows.
		windowOptions.Width, windowOptions.Height = 1280, 800
		windowOptions.MinWidth, windowOptions.MinHeight = 800, 500
		windowOptions.StartState = application.WindowStateFullscreen
	}
	window := wailsApp.Window.NewWithOptions(windowOptions)

	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go showFramelessWindow(window)
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}

// deckLayout reports whether to start with the Steam Deck sized window.
func deckLayout(mode settings.UIMode) bool {
	switch mode {
	case settings.UIModeDeck:
		return true
	case settings.UIModeDesktop:
		return false
	default:
		return platform.IsSteamDeck()
	}
}

// showFramelessWindow shows the main window after preparing its native GTK
// window, which Wails creates asynchronously after startup.
func showFramelessWindow(window *application.WebviewWindow) {
	for range 200 {
		native := application.InvokeSyncWithResult(window.NativeWindow)
		if native != nil {
			application.InvokeSync(func() { platform.PrepareFramelessWindow(native, app.Name) })
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	window.Show()
}

func runWrapper(args []string) int {
	w := &launch.Wrapper{Notify: notify}
	b, err := newBackend()
	if err == nil {
		w.Lib, w.Installer = b.lib, b.installer
	}
	return w.Run(args)
}

func notify(summary, body string) {
	_ = platform.Command("notify-send", "--app-name="+app.Name, summary, body).Run()
}

// migrateDataDir moves data kept under the app's working title to its final
// name, so existing profiles survive the rename.
func migrateDataDir(old, current string) {
	if _, err := os.Stat(current); err == nil {
		return
	}
	if _, err := os.Stat(old); err != nil {
		return
	}
	if err := os.Rename(old, current); err != nil {
		log.Printf("move %s to %s: %v", old, current, err)
	}
}
