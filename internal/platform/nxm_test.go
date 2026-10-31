package platform

import "testing"

func TestDesktopQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/home/u/Hermit/bin/hermit":       "/home/u/Hermit/bin/hermit",
		"/home/u/My Apps/Hermit.AppImage": `"/home/u/My Apps/Hermit.AppImage"`,
		`/opt/$weird "name"`:              `"/opt/\\$weird \\"name\\""`,
	} {
		if got := desktopQuote(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
