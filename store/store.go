// Package store keeps durtle-tui's token and JSON files on disk.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	appName     = "durtle-tui"
	keyringUser = "api-token"
)

// ErrNoToken means no token has been saved yet.
var ErrNoToken = errors.New("no API token saved")

// LoadToken reads the token from the OS keyring, or from the fallback file.
func LoadToken() (string, error) {
	if tok, err := keyring.Get(appName, keyringUser); err == nil {
		return tok, nil
	}
	path, err := tokenPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// SaveToken stores the token in the OS keyring. If there is no usable
// keyring, it writes a file only the current user can read.
func SaveToken(tok string) error {
	if err := keyring.Set(appName, keyringUser, tok); err == nil {
		return nil
	}
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(tok+"\n"), 0o600)
}

func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName, "token"), nil
}

// CacheDir is where synced data lives, e.g. ~/.cache/durtle-tui on Linux.
func CacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

// ReadJSON decodes the file at path into v. A missing file is not an error
// and leaves v untouched.
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// WriteJSON writes v to path atomically: it writes a temp file and renames
// it over the old one, so a crash never leaves a half-written file.
func WriteJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
