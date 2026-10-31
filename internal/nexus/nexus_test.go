package nexus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseLink(t *testing.T) {
	l, err := ParseLink("nxm://SkyrimSpecialEdition/mods/12604/files/35407?key=abc&expires=1700000000&user_id=1")
	if err != nil {
		t.Fatal(err)
	}
	if l != (Link{Game: "skyrimspecialedition", ModID: 12604, FileID: 35407, Key: "abc", Expires: "1700000000"}) {
		t.Errorf("%+v", l)
	}
	for _, bad := range []string{
		"https://www.nexusmods.com/skyrimspecialedition/mods/12604",
		"nxm://skyrimspecialedition/collections/abc/revisions/3",
		"nxm://skyrimspecialedition/mods/x/files/1",
	} {
		if _, err := ParseLink(bad); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

func TestClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Please provide a valid API Key"}`))
			return
		}
		switch r.URL.Path {
		case "/v1/users/validate.json":
			w.Write([]byte(`{"name":"omka","is_premium":false}`))
		case "/v1/games/skyrimspecialedition/mods/12604/files/35407.json":
			w.Write([]byte(`{"name":"SkyUI","version":"5.2SE","file_name":"SkyUI_5_2_SE-12604-5-2SE.7z","size_in_bytes":2693003}`))
		case "/v1/games/skyrimspecialedition/mods/12604/files/35407/download_link.json":
			if r.URL.Query().Get("key") != "abc" {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"message":"You don't have permission to get download links from the API without visting nexusmods.com"}`))
				return
			}
			w.Write([]byte(`[{"name":"Nexus CDN","short_name":"Nexus CDN","URI":"https://cf-files.nexusmods.com/x.7z"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	c := &Client{BaseURL: srv.URL, Key: "secret"}
	if a, err := c.Validate(ctx); err != nil || a.Name != "omka" || a.Premium {
		t.Errorf("validate: %+v %v", a, err)
	}
	l := Link{Game: "skyrimspecialedition", ModID: 12604, FileID: 35407, Key: "abc", Expires: "1"}
	if f, err := c.File(ctx, l); err != nil || f.FileName != "SkyUI_5_2_SE-12604-5-2SE.7z" || f.Size != 2693003 {
		t.Errorf("file: %+v %v", f, err)
	}
	if u, err := c.DownloadURL(ctx, l); err != nil || u != "https://cf-files.nexusmods.com/x.7z" {
		t.Errorf("download: %q %v", u, err)
	}
	l.Key = ""
	if _, err := c.DownloadURL(ctx, l); err == nil {
		t.Error("downloaded without the link's key and no Premium")
	}
	if _, err := (&Client{BaseURL: srv.URL, Key: "wrong"}).Validate(ctx); err == nil {
		t.Error("a wrong key passed")
	}
	if _, err := (&Client{BaseURL: srv.URL}).Validate(ctx); err != ErrNoKey {
		t.Errorf("no key: %v", err)
	}
}
