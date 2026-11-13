package windows

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const maxLayoutBytes = 64 * 1024

type LayoutStore interface {
	Load() ([]Layout, error)
	Save([]Layout) error
}
type LayoutFile struct{ Path string }

func DefaultLayoutStore() LayoutStore {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return LayoutFile{}
		}
		dir = filepath.Join(home, ".local/share")
	}
	if !filepath.IsAbs(dir) {
		return LayoutFile{}
	}
	return LayoutFile{Path: filepath.Join(dir, "omnibar/window-layouts.json")}
}
func (s LayoutFile) Load() ([]Layout, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("layout storage is unavailable")
	}
	f, err := os.OpenFile(s.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return []Layout{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not open saved layouts")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("saved layouts must be a private regular file (0600)")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxLayoutBytes+1))
	if err != nil || len(data) > maxLayoutBytes {
		return nil, fmt.Errorf("saved layout data exceeds its size limit")
	}
	var saved struct {
		Version int      `json:"version"`
		Layouts []Layout `json:"layouts"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&saved) != nil || saved.Version != 1 || d.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("saved layout data is invalid; original file preserved")
	}
	if err := validateLayouts(saved.Layouts); err != nil {
		return nil, fmt.Errorf("saved layout data is invalid; original file preserved")
	}
	return saved.Layouts, nil
}
func (s LayoutFile) Save(layouts []Layout) error {
	if err := validateLayouts(layouts); err != nil {
		return err
	}
	if s.Path == "" {
		return fmt.Errorf("layout storage is unavailable")
	}
	if info, err := os.Lstat(s.Path); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return fmt.Errorf("unsafe saved layout file was preserved")
	}
	data, err := json.Marshal(struct {
		Version int      `json:"version"`
		Layouts []Layout `json:"layouts"`
	}{1, layouts})
	if err != nil || len(data) > maxLayoutBytes {
		return fmt.Errorf("saved layouts exceed 64 KiB")
	}
	dir := filepath.Dir(s.Path)
	if os.MkdirAll(dir, 0700) != nil {
		return fmt.Errorf("could not create layouts directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("layouts directory is unavailable")
	}
	if os.Chmod(dir, 0700) != nil {
		return fmt.Errorf("could not protect layouts directory")
	}
	f, err := os.CreateTemp(dir, ".window-layouts-*")
	if err != nil {
		return fmt.Errorf("could not create layouts file")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return fmt.Errorf("could not write layouts")
	}
	if f.Sync() != nil || f.Close() != nil {
		return fmt.Errorf("could not sync layouts")
	}
	if os.Rename(f.Name(), s.Path) != nil {
		return fmt.Errorf("could not replace layouts")
	}
	return nil
}
