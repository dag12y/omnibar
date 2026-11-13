package apps

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const maxDesktopBytes = 64 * 1024
const maxScanned = 4096

type Desktop struct{}

func ValidID(id string) bool {
	return strings.HasSuffix(id, ".desktop") && len(id) > 8 && len(id) <= 256 && utf8.ValidString(id) && !strings.ContainsAny(id, "/\\") && strings.IndexFunc(id, unicode.IsControl) < 0
}
func roots() []string {
	home := os.Getenv("XDG_DATA_HOME")
	if home == "" {
		if userHome, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(userHome, ".local/share")
		}
	}
	dirs := os.Getenv("XDG_DATA_DIRS")
	if dirs == "" {
		dirs = "/usr/local/share:/usr/share"
	}
	result := []string{}
	for _, dir := range append([]string{home}, strings.Split(dirs, ":")...) {
		if filepath.IsAbs(dir) {
			result = append(result, filepath.Join(dir, "applications"))
		}
	}
	return result
}
func textLabel(s string, n int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if len(s) > n {
		s = s[:n]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}
func localized(values map[string]string, key string) string {
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LC_MESSAGES")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	// Remove the encoding, retaining a locale modifier if present.
	base, modifier, _ := strings.Cut(locale, "@")
	base, _, _ = strings.Cut(base, ".")
	language, _, _ := strings.Cut(base, "_")
	variants := []string{}
	if modifier != "" {
		variants = append(variants, base+"@"+modifier)
	}
	variants = append(variants, base)
	if modifier != "" {
		variants = append(variants, language+"@"+modifier)
	}
	variants = append(variants, language)
	for _, variant := range variants {
		if value := values[key+"["+variant+"]"]; value != "" {
			return value
		}
	}
	return values[key]
}
func parseDesktop(path string) map[string]string {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxDesktopBytes {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(f, maxDesktopBytes+1))
	if err != nil || len(data) > maxDesktopBytes || !utf8.Valid(data) {
		return nil
	}
	values := map[string]string{}
	inEntry := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.NewReplacer(`\s`, " ", `\n`, "\n", `\t`, "\t", `\r`, "\r", `\\`, `\`).Replace(strings.TrimSpace(value))
	}
	return values
}
func shown(values map[string]string) bool {
	if values["Type"] != "Application" || values["Hidden"] == "true" || values["NoDisplay"] == "true" || localized(values, "Name") == "" {
		return false
	}
	if values["Exec"] == "" && values["DBusActivatable"] != "true" {
		return false
	}
	desktops := strings.Split(os.Getenv("XDG_CURRENT_DESKTOP"), ":")
	contains := func(list string) bool {
		for _, item := range strings.Split(list, ";") {
			for _, d := range desktops {
				if item != "" && item == d {
					return true
				}
			}
		}
		return false
	}
	if values["OnlyShowIn"] != "" && !contains(values["OnlyShowIn"]) {
		return false
	}
	if contains(values["NotShowIn"]) {
		return false
	}
	if executable := values["TryExec"]; executable != "" {
		if _, err := exec.LookPath(executable); err != nil {
			return false
		}
	}
	return true
}
func catalog(ctx context.Context) ([]App, map[string]string, bool, error) {
	entries := []App{}
	paths := map[string]string{}
	seen := map[string]bool{}
	count := 0
	truncated := false
	for _, root := range roots() {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			count++
			if count > maxScanned {
				truncated = true
				return filepath.SkipAll
			}
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			id := strings.ReplaceAll(rel, string(filepath.Separator), "-")
			if !ValidID(id) || seen[id] {
				return nil
			}
			seen[id] = true
			values := parseDesktop(path)
			if !shown(values) {
				return nil
			}
			name := textLabel(localized(values, "Name"), 128)
			if name == "" {
				return nil
			}
			entries = append(entries, App{ID: id, Name: name, Description: textLabel(localized(values, "Comment"), 128), Keywords: textLabel(localized(values, "GenericName")+" "+localized(values, "Keywords"), 128)})
			paths[id] = path
			return nil
		})
		if err != nil {
			return nil, nil, false, fmt.Errorf("could not read installed applications")
		}
		if count > maxScanned {
			break
		}
	}
	return entries, paths, truncated, nil
}
func (Desktop) Read(ctx context.Context) ([]App, bool, error) {
	a, _, truncated, err := catalog(ctx)
	return a, truncated, err
}
func (Desktop) Launch(ctx context.Context, id string) error {
	if !ValidID(id) {
		return fmt.Errorf("invalid application ID")
	}
	_, paths, _, err := catalog(ctx)
	if err != nil {
		return err
	}
	path, ok := paths[id]
	if !ok {
		return fmt.Errorf("application is no longer available; refresh and try again")
	}
	cmd := exec.CommandContext(ctx, "gio", "launch", path)
	// Do not connect pipes to descendants or retain app output/private arguments.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not launch application; check gio and your desktop session")
	}
	return nil
}
