package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hermit/internal/library"
)

func TestDownloads(t *testing.T) {
	var fileHits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/files/SkyUI.7z" {
			fileHits.Add(1)
			w.Write([]byte("archive"))
			return
		}
		if r.Header.Get("apikey") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/users/validate.json":
			w.Write([]byte(`{"name":"omka","is_premium":false}`))
		case "/v1/games/skyrimspecialedition/mods/12604/files/35407.json":
			w.Write([]byte(`{"name":"SkyUI main","version":"5.2SE","file_name":"SkyUI.7z","size_in_bytes":7}`))
		case "/v1/games/skyrimspecialedition/mods/12604.json":
			w.Write([]byte(`{"name":"SkyUI","author":"schlangster","version":"5.2SE"}`))
		case "/v1/games/skyrimspecialedition/mods/12604/files/35407/download_link.json":
			w.Write([]byte(`[{"name":"CDN","URI":"` + srv.URL + `/files/SkyUI.7z"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	lib, err := library.New(filepath.Join(tmp, "lib"), nil)
	if err != nil {
		t.Fatal(err)
	}
	gamePath := filepath.Join(tmp, "Skyrim Special Edition")
	writeFiles(t, gamePath, map[string]string{"SkyrimSE.exe": "", "Data/Skyrim.esm": ""})
	game, err := lib.AddGame("Skyrim", gamePath)
	if err != nil {
		t.Fatal(err)
	}
	s := NewDownloadService(lib, filepath.Join(tmp, "lib"), "test")
	s.baseURL = srv.URL
	const link = "nxm://skyrimspecialedition/mods/12604/files/35407?key=k&expires=1"

	if err := s.HandleNXM(link); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("without a key: %v", err)
	}
	if _, err := s.SetAPIKey(context.Background(), "wrong"); err == nil {
		t.Error("kept a key Nexus Mods refused")
	}
	account, err := s.SetAPIKey(context.Background(), "secret")
	if err != nil || account.Name != "omka" || !s.Account().HasKey {
		t.Fatalf("account %+v %v", account, err)
	}
	if fi, _ := os.Stat(s.keyFile()); fi.Mode().Perm() != 0o600 {
		t.Errorf("key file mode %v", fi.Mode().Perm())
	}
	if err := s.HandleNXM("nxm://fallout4/mods/1/files/2?key=k&expires=1"); err == nil || !strings.Contains(err.Error(), "Fallout 4") {
		t.Errorf("a game not added: %v", err)
	}

	wait := func() Download {
		t.Helper()
		for range 200 {
			list := s.List()
			if d := list[len(list)-1]; d.Status != DownloadQueued && d.Status != DownloadRunning {
				return d
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("the download did not finish")
		return Download{}
	}
	if err := s.HandleNXM(link); err != nil {
		t.Fatal(err)
	}
	d := wait()
	profileDir, _ := lib.ProfileDir(game.ID, game.ActiveProfile)
	if d.Status != DownloadDone || d.Name != "SkyUI" || d.Path != filepath.Join(profileDir, "downloads", "SkyUI.7z") || d.ProfileID != game.ActiveProfile {
		t.Fatalf("download %+v", d)
	}
	if data, _ := os.ReadFile(d.Path); string(data) != "archive" {
		t.Errorf("file %q", data)
	}
	if got := s.TakeFinished(); len(got) != 1 || got[0].ID != d.ID {
		t.Errorf("taken %+v", got)
	}
	if got := s.TakeFinished(); len(got) != 0 {
		t.Errorf("taken twice: %+v", got)
	}

	// The profile has the archive: it is not downloaded again.
	if err := s.HandleNXM(link); err != nil {
		t.Fatal(err)
	}
	if d := wait(); d.Status != DownloadDone || fileHits.Load() != 1 {
		t.Errorf("again: %+v, %d downloads", d, fileHits.Load())
	}
}
