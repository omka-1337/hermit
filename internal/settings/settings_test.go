package settings

import "testing"

func TestCompleteSetupPersists(t *testing.T) {
	root := t.TempDir()
	st, err := NewStore(root).Get()
	if err != nil || st.SetupCompleted {
		t.Fatalf("fresh store: %+v, %v", st, err)
	}
	if _, err := NewStore(root).CompleteSetup(); err != nil {
		t.Fatal(err)
	}
	if st, _ := NewStore(root).Get(); !st.SetupCompleted {
		t.Fatal("setup flag not persisted")
	}
}

func TestUpdateKeepsSetupFlag(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.CompleteSetup(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Update(Settings{AllowNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if !st.SetupCompleted || !st.AllowNSFW {
		t.Fatalf("after update: %+v", st)
	}
}

func TestUIModeDefaultsToAuto(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if st, _ := store.Get(); st.UIMode != UIModeAuto {
		t.Errorf("fresh settings: %q", st.UIMode)
	}
	if _, err := store.Update(Settings{UIMode: UIModeDeck}); err != nil {
		t.Fatal(err)
	}
	if st, _ := NewStore(root).Get(); st.UIMode != UIModeDeck {
		t.Errorf("stored mode: %q", st.UIMode)
	}
	if _, err := store.Update(Settings{UIMode: "nonsense"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := NewStore(root).Get(); st.UIMode != UIModeAuto {
		t.Errorf("unknown mode should fall back to auto: %q", st.UIMode)
	}
}

func TestLastGameSurvivesUpdate(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetLastGame("skyrim"); err != nil {
		t.Fatal(err)
	}
	// The settings page saves what it was given before the game was opened.
	if _, err := store.Update(Settings{AllowNSFW: true}); err != nil {
		t.Fatal(err)
	}
	if st, _ := store.Get(); st.LastGame != "skyrim" || !st.AllowNSFW {
		t.Fatalf("after update: %+v", st)
	}
}

func TestPanelWidthsSurviveUpdate(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.SetPanelWidth("sidebar", 300); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPanelWidth("plugins", 420); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Get()
	st.AllowNSFW = true
	st.PanelWidths = nil
	if _, err := s.Update(st); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get()
	if got.PanelWidths["sidebar"] != 300 || got.PanelWidths["plugins"] != 420 || !got.AllowNSFW {
		t.Errorf("after an update: %+v", got)
	}
}
