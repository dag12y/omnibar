package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := Run([]string{"-help"}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run(-help) returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("expected usage output, got: %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("expected no stderr output, got: %q", errOut.String())
	}
}

func TestRunDoubleDashHelp(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"--help"}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run(--help) returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("expected usage output, got: %q", out.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() with no args returned error: %v", err)
	}
	if !strings.Contains(out.String(), "starting") {
		t.Errorf("expected startup message, got: %q", out.String())
	}
}

func TestRunParseError(t *testing.T) {
	// flag.Parse returns an error for unknown flags that look like
	// flags (start with - but aren't recognized). Usage goes to stderr.
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := Run([]string{"-unknown-flag"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if out.Len() != 0 {
		t.Errorf("expected no stdout output, got: %q", out.String())
	}
	if errOut.Len() == 0 {
		t.Error("expected usage output on stderr for parse error")
	}
}

func TestIsHelpRequest(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"-help"}, true},
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{[]string{"--h"}, true},
		{[]string{}, false},
		{[]string{"foo"}, false},
		{[]string{"-help", "foo"}, true},
	}

	for _, c := range cases {
		if got := isHelpRequest(c.args); got != c.want {
			t.Errorf("isHelpRequest(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestArgsWithoutProgram(t *testing.T) {
	// No path separator: args returned unchanged.
	args := argsWithoutProgram([]string{"-help", "foo"})
	if len(args) != 2 || args[0] != "-help" || args[1] != "foo" {
		t.Errorf("argsWithoutProgram(%v) = %v, expected unchanged", []string{"-help", "foo"}, args)
	}

	// With path separator: leading program name removed.
	args = argsWithoutProgram([]string{"/usr/bin/omnibar", "-help"})
	if len(args) != 1 || args[0] != "-help" {
		t.Errorf("argsWithoutProgram(%v) = %v, expected [%q]", []string{"/usr/bin/omnibar", "-help"}, args, "-help")
	}
}
