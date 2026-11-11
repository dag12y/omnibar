// Package packaging holds static checks that keep the Arch packaging files
// (pkgbuild/, systemd/) consistent with the code they ship.
package packaging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc"
)

func readRepoFile(t *testing.T, rel ...string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, rel...)...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(rel...), err)
	}
	return string(data)
}

func TestPKGBUILDVersionMatchesBackend(t *testing.T) {
	pkgbuild := readRepoFile(t, "pkgbuild", "PKGBUILD")

	m := regexp.MustCompile(`(?m)^pkgver=(\S+)$`).FindStringSubmatch(pkgbuild)
	if m == nil {
		t.Fatal("pkgbuild/PKGBUILD has no pkgver")
	}
	if m[1] != ipc.ServerVersion {
		t.Errorf("PKGBUILD pkgver = %q, backend version (ipc.ServerVersion) = %q; keep them in sync", m[1], ipc.ServerVersion)
	}
}

func TestPKGBUILDInstallsEveryShippedFile(t *testing.T) {
	pkgbuild := readRepoFile(t, "pkgbuild", "PKGBUILD")

	for _, want := range []string{
		`"$pkgdir/usr/bin/omnibar"`,
		`"$pkgdir/usr/bin/omnibar-ui"`,
		`"$pkgdir/usr/lib/systemd/user/omnibar.service"`,
		`"$pkgdir/usr/share/omnibar/qml/shell.qml"`,
		`ui/qml/components/*`,
		`config/config.toml.example`,
		`"$pkgdir/usr/share/licenses/$pkgname/LICENSE"`,
	} {
		if !strings.Contains(pkgbuild, want) {
			t.Errorf("PKGBUILD does not install %s", want)
		}
	}

	// Tests are not shipped.
	if strings.Contains(pkgbuild, "ui/qml/tests") {
		t.Error("PKGBUILD must not install QML tests")
	}
}

func TestLauncherPointsAtInstalledQML(t *testing.T) {
	launcher := readRepoFile(t, "pkgbuild", "omnibar-ui")

	if !strings.HasPrefix(launcher, "#!/bin/sh\n") {
		t.Error("omnibar-ui must start with a #!/bin/sh shebang")
	}
	if !strings.Contains(launcher, "exec quickshell -n -p /usr/share/omnibar/qml/shell.qml") {
		t.Errorf("omnibar-ui does not launch the installed shell.qml:\n%s", launcher)
	}
}

func TestSystemdUnit(t *testing.T) {
	// A plain user unit: not a template (@) and one backend per user.
	if _, err := os.Stat(filepath.Join("..", "..", "systemd", "omnibar@.service")); err == nil {
		t.Error("omnibar must be a single per-user service, not a template unit")
	}

	unit := readRepoFile(t, "systemd", "omnibar.service")
	for _, want := range []string{
		"ExecStart=/usr/bin/omnibar\n",
		"Type=simple\n",
		"Restart=on-failure\n",
		"WantedBy=default.target\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("omnibar.service is missing %q", strings.TrimSpace(want))
		}
	}
	if strings.Contains(unit, "%i") || strings.Contains(unit, "%I") {
		t.Error("omnibar.service must not use template specifiers")
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "config", "config.toml.example")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config.toml.example: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config.toml.example: %v", err)
	}
}

func TestNoSelfUpdater(t *testing.T) {
	// ADR-0008: updates are handled by pacman/OPR only.
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "build", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && strings.Contains(strings.ToLower(filepath.Base(path)), "updater") {
			t.Errorf("self-updater code must not exist: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
}
