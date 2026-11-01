//go:build !linux || !cgo || gtk3

package platform

import (
	"errors"
	"unsafe"
)

// Browser is a web view of its own inside the app's window; only GTK 4 on
// Linux has one.
type Browser struct {
	OnNXM    func(link string)
	OnChange func()
	OnCrash  func(reason string)
}

// BrowserState is what the browser shows.
type BrowserState struct {
	URI       string  `json:"uri"`
	Title     string  `json:"title"`
	CanGoBack bool    `json:"canGoBack"`
	Loading   bool    `json:"loading"`
	Progress  float64 `json:"progress"`
}

func NewBrowser(unsafe.Pointer, string, string) (*Browser, error) {
	return nil, errors.New("a browser inside the window needs GTK 4 on Linux")
}

func (b *Browser) Place(x, y, w, h int) {}
func (b *Browser) Hide()                {}
func (b *Browser) Load(string)          {}
func (b *Browser) Back()                {}
func (b *Browser) Reload()              {}
func (b *Browser) State() BrowserState  { return BrowserState{} }
