package service

import (
	"errors"
	"testing"
)

func TestLogContainerPresenceResult(t *testing.T) {
	exitErr := errors.New("exit status 1")
	cases := []struct {
		name    string
		kind    string
		output  string
		err     error
		missing bool
		fails   bool
	}{
		{"container exists", "container", "abc123\n", nil, false, false},
		{"container missing", "container", "Error: No such container: web", exitErr, true, true},
		{"container missing object", "container", "Error: No such object: web", exitErr, true, true},
		{"docker failure", "container", "Cannot connect to the Docker daemon", exitErr, false, true},
		{"compose has containers", "compose", "abc\ndef\n", nil, false, false},
		{"compose without containers", "compose", "  \n", nil, true, true},
		{"compose command failure", "compose", "no such file", exitErr, false, true},
	}
	for _, c := range cases {
		err := logContainerPresenceResult(c.kind, c.output, c.err)
		if (err != nil) != c.fails {
			t.Fatalf("%s: err = %v", c.name, err)
		}
		if errors.Is(err, errLogContainerMissing) != c.missing {
			t.Fatalf("%s: missing = %v, err = %v", c.name, !c.missing, err)
		}
	}
}
