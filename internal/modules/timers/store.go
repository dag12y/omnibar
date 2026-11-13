package timers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf8"
)

type FileStore struct{ Path string }

func DefaultStore() Store {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return FileStore{}
		}
		dir = filepath.Join(home, ".local/share")
	}
	if !filepath.IsAbs(dir) {
		return FileStore{}
	}
	return FileStore{Path: filepath.Join(dir, "omnibar/timers.json")}
}
func (s FileStore) Load() (Data, error) {
	if s.Path == "" {
		return Data{}, fmt.Errorf("timer storage is unavailable")
	}
	f, err := os.OpenFile(s.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return Data{Version: 1, Revision: 1, Timers: []Record{}}, nil
	}
	if err != nil {
		return Data{}, fmt.Errorf("could not open saved timers")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return Data{}, fmt.Errorf("saved timers must be a private regular file (0600)")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil || len(data) > MaxFileBytes || !utf8.Valid(data) {
		return Data{}, fmt.Errorf("saved timer data exceeds its size limit")
	}
	var saved Data
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&saved) != nil || saved.Version != 1 || d.Decode(new(any)) != io.EOF {
		return Data{}, fmt.Errorf("saved timer data is invalid; original file preserved")
	}
	if err := validateData(saved); err != nil {
		return Data{}, fmt.Errorf("saved timer data is invalid; original file preserved")
	}
	return saved, nil
}
func (s FileStore) Save(data Data) error {
	if err := validateData(data); err != nil {
		return err
	}
	if s.Path == "" {
		return fmt.Errorf("timer storage is unavailable")
	}
	if info, err := os.Lstat(s.Path); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return fmt.Errorf("unsafe saved timer file was preserved")
	}
	encoded, err := json.Marshal(data)
	if err != nil || len(encoded) > MaxFileBytes {
		return fmt.Errorf("saved timers exceed 64 KiB")
	}
	dir := filepath.Dir(s.Path)
	if os.MkdirAll(dir, 0700) != nil {
		return fmt.Errorf("could not create timers directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("timers directory is unavailable")
	}
	if os.Chmod(dir, 0700) != nil {
		return fmt.Errorf("could not protect timers directory")
	}
	f, err := os.CreateTemp(dir, ".timers-*")
	if err != nil {
		return fmt.Errorf("could not create timers file")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(encoded); err != nil {
		return fmt.Errorf("could not write timers")
	}
	if f.Sync() != nil || f.Close() != nil {
		return fmt.Errorf("could not sync timers")
	}
	if os.Rename(f.Name(), s.Path) != nil {
		return fmt.Errorf("could not replace timers")
	}
	return nil
}
