package clipboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const MaxFavorites = 20
const MaxFavoriteBytes = 32 * 1024
const maxFavoritesFileBytes = 256 * 1024

// FavoritesStore stores only explicitly saved entries, never ordinary history.
type FavoritesStore interface {
	Load() ([]Entry, error)
	Save([]Entry) error
}
type FileStore struct{ Path string }

func DefaultFavoritesStore() FavoritesStore {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return FileStore{}
		}
		base = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(base) {
		return FileStore{}
	}
	return FileStore{Path: filepath.Join(base, "omnibar", "clipboard-favorites.json")}
}

type favoritesFile struct {
	Version int          `json:"version"`
	Entries []savedEntry `json:"entries"`
}
type savedEntry struct {
	Text     string `json:"text"`
	CopiedAt string `json:"copied_at"`
}

func validateFavorites(entries []Entry) error {
	if len(entries) > MaxFavorites {
		return fmt.Errorf("favorite limit reached (20 items)")
	}
	total := 0
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !validText(entry.Text) || seen[entry.Text] {
			return fmt.Errorf("invalid favorites data")
		}
		if _, err := time.Parse(time.RFC3339Nano, entry.CopiedAt); err != nil {
			return fmt.Errorf("invalid favorite timestamp")
		}
		seen[entry.Text] = true
		total += len(entry.Text)
	}
	if total > MaxFavoriteBytes {
		return fmt.Errorf("favorites text limit reached (32 KiB)")
	}
	return nil
}
func (s FileStore) Load() ([]Entry, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("favorites storage path is unavailable")
	}
	// Refuse symlinks and non-private/non-regular files instead of following them.
	file, err := os.OpenFile(s.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not open saved favorites")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("saved favorites must be a private regular file (mode 0600)")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFavoritesFileBytes+1))
	if err != nil || len(data) > maxFavoritesFileBytes {
		return nil, fmt.Errorf("could not read bounded favorites data")
	}
	var saved favoritesFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&saved) != nil || saved.Version != 1 || decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("saved favorites format is invalid; original file was preserved")
	}
	entries := make([]Entry, 0, len(saved.Entries))
	for _, item := range saved.Entries {
		entries = append(entries, Entry{Text: item.Text, CopiedAt: item.CopiedAt, Favorite: true})
	}
	if err := validateFavorites(entries); err != nil {
		return nil, err
	}
	return entries, nil
}
func (s FileStore) Save(entries []Entry) error {
	if err := validateFavorites(entries); err != nil {
		return err
	}
	if s.Path == "" {
		return fmt.Errorf("favorites storage path is unavailable")
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("could not create private favorites directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("favorites directory must be a regular directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("could not protect favorites directory")
	}
	saved := favoritesFile{Version: 1, Entries: make([]savedEntry, 0, len(entries))}
	for _, entry := range entries {
		saved.Entries = append(saved.Entries, savedEntry{Text: entry.Text, CopiedAt: entry.CopiedAt})
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("could not encode favorites")
	}
	file, err := os.CreateTemp(dir, ".clipboard-favorites-*")
	if err != nil {
		return fmt.Errorf("could not create favorites file")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("could not write favorites")
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("could not sync favorites")
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("could not close favorites")
	}
	if err := os.Rename(file.Name(), s.Path); err != nil {
		return fmt.Errorf("could not replace saved favorites")
	}
	return nil
}
