package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestTokenKeyring(t *testing.T) {
	keyring.MockInit()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := LoadToken(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty store: err = %v, want ErrNoToken", err)
	}
	if err := SaveToken("abc"); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadToken(); err != nil || got != "abc" {
		t.Errorf("LoadToken = %q, %v", got, err)
	}
}

func TestTokenFileFallback(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keyring here"))
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := SaveToken("abc"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "durtle-tui", "token"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %o, want 600", perm)
	}
	if got, err := LoadToken(); err != nil || got != "abc" {
		t.Errorf("LoadToken = %q, %v", got, err)
	}
}

func TestJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "x.json")
	v := map[int]string{1: "a"}
	if err := ReadJSON(path, &v); err != nil || v[1] != "a" {
		t.Fatalf("missing file should be a no-op: %v %v", v, err)
	}
	if err := WriteJSON(path, map[int]string{2: "b"}); err != nil {
		t.Fatal(err)
	}
	var got map[int]string
	if err := ReadJSON(path, &got); err != nil || got[2] != "b" {
		t.Errorf("round trip = %v, %v", got, err)
	}
	os.WriteFile(path, []byte("{not json"), 0o600)
	if err := ReadJSON(path, &got); err == nil {
		t.Error("corrupt file should return an error")
	}
}
