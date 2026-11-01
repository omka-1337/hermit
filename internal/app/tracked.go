package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"hermit/internal/modinstall"
)

// tracked is a download DownloadService lists and does not make itself: a
// mod with its dependencies, or a modpack, InstallService downloads and
// installs from Thunderstore, as one entry however many packages it takes.
type tracked struct {
	s *DownloadService
	d *Download
}

// track lists a download of InstallService's for a profile of a game, under
// name, until finish; key names the package, so that the plan that
// downloads it and the install that unpacks it are the same entry. Cancel
// calls cancel.
func (s *DownloadService) track(gameID, profileID, key, name string, cancel context.CancelFunc) *tracked {
	gameName := gameID
	if g, err := s.lib.GetGame(gameID); err == nil {
		gameName = g.Name
	}
	s.mu.Lock()
	// The install after a plan carries on its entry.
	i := slices.IndexFunc(s.downloads, func(d *Download) bool {
		return d.key == key && d.Status != DownloadFailed && d.Status != DownloadCancelled
	})
	var d *Download
	if i >= 0 {
		d = s.downloads[i]
		s.downloads = slices.Delete(s.downloads, i, i+1)
	} else {
		s.nextID++
		d = &Download{ID: fmt.Sprint(s.nextID), GameID: gameID, GameName: gameName, ProfileID: profileID, Name: name, Source: "thunderstore", key: key}
	}
	// Installed by InstallService, not by the main window's queue.
	d.taken = true
	d.Status, d.Stage, d.Progress, d.Error, d.cancel = DownloadRunning, StageDownload, 0, "", cancel
	s.downloads = append(s.downloads, d)
	s.mu.Unlock()
	s.changed()
	return &tracked{s: s, d: d}
}

// progress takes in a step of the install: while downloading, how many of
// the packages known are there and how far the current one is; while
// installing, which of them is being put in place.
func (t *tracked) progress(p modinstall.Progress) {
	t.s.mu.Lock()
	d := t.d
	if d.Status != DownloadRunning {
		t.s.mu.Unlock()
		return
	}
	steps := float64(max(1, p.Steps))
	switch p.Stage {
	case modinstall.StageInstall:
		d.Stage = StageUnpack
		d.Progress = min(1, float64(max(0, p.Step-1))/steps)
		d.Done, d.Size = 0, 0
	default:
		d.Stage = StageDownload
		part := 0.0
		if p.Total > 0 {
			part = float64(p.Done) / float64(p.Total)
		}
		d.Progress = min(1, (float64(p.Step)+part)/steps)
		d.Done, d.Size = p.Done, p.Total
	}
	t.s.mu.Unlock()
	t.s.changed()
}

// finish ends the entry with how the install went.
func (t *tracked) finish(err error) {
	t.s.mu.Lock()
	d := t.d
	switch {
	case d.Status == DownloadCancelled || errors.Is(err, context.Canceled):
		d.Status = DownloadCancelled
	case err != nil:
		d.Status, d.Error = DownloadFailed, err.Error()
	default:
		d.Status, d.Progress = DownloadDone, 1
	}
	d.cancel = nil
	t.s.mu.Unlock()
	t.s.changed()
}
