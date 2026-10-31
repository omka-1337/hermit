package main

import (
	"reflect"
	"testing"
)

func TestParsePrepareArgs(t *testing.T) {
	gameID, appID, pid, command, err := parsePrepareArgs([]string{
		"--game", "lethal-company", "--appid", "1966720", "--pid", "4242", "--", "wine", "Game.exe",
	})
	if err != nil {
		t.Fatalf("parsePrepareArgs: %v", err)
	}
	if gameID != "lethal-company" || appID != "1966720" || pid != 4242 {
		t.Errorf("got game=%q appid=%q pid=%d", gameID, appID, pid)
	}
	if want := []string{"wine", "Game.exe"}; !reflect.DeepEqual(command, want) {
		t.Errorf("command = %q, want %q", command, want)
	}
}

func TestParsePrepareArgsRejects(t *testing.T) {
	for _, args := range [][]string{
		{},                         // nothing at all
		{"--game", "x"},            // no command
		{"--game"},                 // value missing
		{"--"},                     // empty command
		{"nonsense", "--", "wine"}, // unknown argument
	} {
		if _, _, _, _, err := parsePrepareArgs(args); err == nil {
			t.Errorf("parsePrepareArgs(%q) accepted bad arguments", args)
		}
	}
}

// An empty pid, as the script passes when it has none, leaves this process as
// the owner of the session rather than making it 0.
func TestParsePrepareArgsKeepsOwnPID(t *testing.T) {
	_, _, pid, _, err := parsePrepareArgs([]string{"--pid", "", "--", "wine"})
	if err != nil {
		t.Fatalf("parsePrepareArgs: %v", err)
	}
	if pid <= 0 {
		t.Errorf("pid = %d, want this process", pid)
	}
}

func TestChangedEnv(t *testing.T) {
	current := []string{"HOME=/home/x", "WINEDLLOVERRIDES=d3d11=n"}
	want := []string{"HOME=/home/x", "WINEDLLOVERRIDES=d3d11=n;winhttp=n,b", "NEW=1"}
	got := changedEnv(current, want)
	expect := []string{"WINEDLLOVERRIDES=d3d11=n;winhttp=n,b", "NEW=1"}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("changedEnv = %q, want %q", got, expect)
	}
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"simple":       "'simple'",
		"with space":   "'with space'",
		"it's":         `'it'\''s'`,
		"$(rm -rf /)":  "'$(rm -rf /)'",
		"/path/run.sh": "'/path/run.sh'",
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}
