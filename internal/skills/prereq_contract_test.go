package skills

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrerequisiteInstallersWithFixtures(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	script := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := installGoogleCLIDarwinBrew(false); err == nil {
		t.Fatal("missing brew accepted")
	}
	script("brew", "exit 0")
	if res, err := installGoogleCLIDarwinBrew(true); err != nil || len(res.Messages) != 2 {
		t.Fatalf("brew plan: %+v %v", res, err)
	}
	if _, err := installGoogleCLIDarwinBrew(false); err != nil {
		t.Fatal(err)
	}
	script("brew", "printf 'fixture failure' >&2; exit 1")
	if _, err := installGoogleCLIDarwinBrew(false); err == nil {
		t.Fatal("brew failure hidden")
	}
	for _, name := range []string{"apt-get", "sudo", "gpg"} {
		if _, err := installGoogleCLILinuxAPT(false); err == nil {
			t.Fatalf("missing %s accepted", name)
		}
		script(name, "exit 0")
	}
	if res, err := installGoogleCLILinuxAPT(true); err != nil || len(res.Messages) < 5 {
		t.Fatalf("apt plan: %+v %v", res, err)
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("fixture key"))}, nil
	})
	if _, err := installGoogleCLILinuxAPT(false); err == nil || !strings.Contains(err.Error(), "empty output") {
		t.Fatalf("empty dearmor accepted: %v", err)
	}
	script("gpg", "printf 'dearmored fixture'")
	if _, err := installGoogleCLILinuxAPT(false); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"exit 1", "printf 'fixture failure' >&2; exit 1"} {
		script("sudo", body)
		if _, err := installGoogleCLILinuxAPT(false); err == nil {
			t.Fatal("sudo failure hidden")
		}
		if err := runCommandWithStdin(time.Second, []byte("fixture"), "sudo"); err == nil {
			t.Fatal("stdin command failure hidden")
		}
		if err := runCommand(time.Second, "sudo"); err == nil {
			t.Fatal("command failure hidden")
		}
	}
	script("gpg", "exit 1")
	if _, err := dearmorGPGKey([]byte("fixture")); err == nil {
		t.Fatal("gpg failure hidden")
	}
	for _, tc := range []struct {
		status int
		body   string
		err    error
	}{{403, "denied", nil}, {200, "", nil}, {0, "", errors.New("offline")}} {
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			if tc.err != nil {
				return nil, tc.err
			}
			return &http.Response{StatusCode: tc.status, Status: "fixture", Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		if _, err := fetchGoogleCloudAPTKey(); err == nil {
			t.Fatal("key fetch error hidden")
		}
	}
	script("gcloud", "exit 0")
	if res, err := InstallPrerequisite("gcloud", false); err != nil || !res.Installed {
		t.Fatalf("existing prerequisite: %+v %v", res, err)
	}
}
