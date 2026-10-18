package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/skills"
	"github.com/spf13/cobra"
)

type cliTransport func(*http.Request) (*http.Response, error)

func (f cliTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSkillManagementCommandContracts(t *testing.T) {
	root := commandHome(t)
	t.Setenv("PATH", root)
	// Direct command calls keep parsed Cobra flag state out of other tests.
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	call := func(c *cobra.Command, args ...string) string {
		t.Helper()
		buf.Reset()
		if err := c.RunE(cmd, args); err != nil {
			t.Fatalf("%s %v: %v", c.Name(), args, err)
		}
		return buf.String()
	}
	original := []bool{skillsVerifyJSON, skillsInstallJSON, skillsUpdateJSON, skillsApproveWarnings, skillsPrereqDryRun, skillsPrereqJSON, skillsPrereqYes, skillsAuthJSON}
	t.Cleanup(func() {
		skillsVerifyJSON, skillsInstallJSON, skillsUpdateJSON, skillsApproveWarnings, skillsPrereqDryRun, skillsPrereqJSON, skillsPrereqYes, skillsAuthJSON = original[0], original[1], original[2], original[3], original[4], original[5], original[6], original[7]
	})
	if out := call(skillsEnableSkillCmd, "google-cli"); !strings.Contains(out, "Prerequisite missing") {
		t.Fatal(out)
	}
	if out := call(skillsDisableSkillCmd, "google-cli"); !strings.Contains(out, "disabled") {
		t.Fatal(out)
	}
	for _, c := range []*cobra.Command{skillsEnableSkillCmd, skillsDisableSkillCmd, skillsVerifyCmd} {
		if err := c.RunE(cmd, []string{" "}); err == nil {
			t.Fatal("blank target accepted")
		}
	}
	src := filepath.Join(root, "source")
	if err := os.MkdirAll(src, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: fixture-skill\ndescription: Fixture for command tests\n---\n# Fixture\nRead local notes.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL-POLICY.json"), []byte(`{"version":"v1","execution":{"network":false,"readOnlyWorkspace":true,"allowCommands":["printf"],"timeoutSeconds":5,"maxOutputBytes":1024}}`), 0600); err != nil {
		t.Fatal(err)
	}
	skillsApproveWarnings = true
	for _, asJSON := range []bool{false, true} {
		skillsVerifyJSON = asJSON
		out := call(skillsVerifyCmd, src)
		if !strings.Contains(out, "fixture-skill") {
			t.Fatal(out)
		}
		skillsInstallJSON = asJSON
		out = call(skillsInstallCmd, src)
		if !strings.Contains(out, "fixture-skill") {
			t.Fatal(out)
		}
		skillsUpdateJSON = asJSON
		out = call(skillsUpdateCmd, "fixture-skill")
		if !strings.Contains(out, "fixture-skill") {
			t.Fatal(out)
		}
	}
	skillsPrereqJSON = false
	if out := call(skillsPrereqCheckCmd, "google-cli"); !strings.Contains(out, "missing") {
		t.Fatal(out)
	}
	if err := skillsPrereqCheckCmd.RunE(cmd, []string{"unsupported"}); err == nil {
		t.Fatal("unsupported prerequisite accepted")
	}
	skillsPrereqYes = false
	skillsPrereqDryRun = false
	if err := skillsPrereqInstallCmd.RunE(cmd, []string{"google-cli"}); err == nil {
		t.Fatal("missing install confirmation accepted")
	}
	for _, name := range []string{"brew", "apt-get", "sudo", "gpg", "gcloud"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, asJSON := range []bool{false, true} {
		skillsPrereqJSON = asJSON
		call(skillsPrereqCheckCmd, "google-cli")
		skillsPrereqYes = true
		call(skillsPrereqInstallCmd, "google-cli")
		skillsPrereqDryRun = true
		call(skillsPrereqInstallCmd, "google-cli")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Skills.Enabled = true
	cfg.Skills.Entries["fixture-skill"] = config.SkillEntryConfig{Enabled: true}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := skillsUpdateCmd.RunE(cmd, []string{"missing-skill"}); err == nil {
		t.Fatal("missing install accepted")
	}
	if err := skillsInstallCmd.RunE(cmd, []string{"/missing/skill"}); err == nil {
		t.Fatal("missing source accepted")
	}
}

func TestSkillOAuthCommandContracts(t *testing.T) {
	commandHome(t)
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = cliTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"fixture-access","refresh_token":"fixture-refresh","expires_in":3600,"scope":"openid"}`))}, nil
	})
	original := []string{skillsAuthProfile, skillsAuthClientID, skillsAuthClientSecret, skillsAuthRedirectURI, skillsAuthScopes, skillsAuthAccess, skillsAuthTenantID, skillsAuthCode, skillsAuthState, skillsAuthCallbackURL}
	oldJSON := skillsAuthJSON
	t.Cleanup(func() {
		skillsAuthProfile, skillsAuthClientID, skillsAuthClientSecret, skillsAuthRedirectURI, skillsAuthScopes, skillsAuthAccess, skillsAuthTenantID, skillsAuthCode, skillsAuthState, skillsAuthCallbackURL = original[0], original[1], original[2], original[3], original[4], original[5], original[6], original[7], original[8], original[9]
		skillsAuthJSON = oldJSON
	})
	skillsAuthProfile = "fixture"
	skillsAuthClientID = "fixture-client"
	skillsAuthClientSecret = "fixture-secret"
	skillsAuthRedirectURI = "http://localhost/callback"
	skillsAuthAccess = "all"
	skillsAuthScopes = ""
	skillsAuthCallbackURL = ""
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	for _, id := range []string{"google-workspace", "m365"} {
		skillsAuthJSON = true
		buf.Reset()
		if err := skillsAuthStartCmd.RunE(cmd, []string{id}); err != nil {
			t.Fatal(err)
		}
		var started skills.OAuthStartResult
		if err := json.Unmarshal(buf.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
		skillsAuthCode = ""
		skillsAuthState = ""
		skillsAuthCallbackURL = "http://localhost/callback?code=fixture-code&state=" + started.State
		buf.Reset()
		if err := skillsAuthCompleteCmd.RunE(cmd, []string{id}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "tokenPath") {
			t.Fatal(buf.String())
		}
		skillsAuthJSON = false
		skillsAuthCallbackURL = ""
		buf.Reset()
		if err := skillsAuthStartCmd.RunE(cmd, []string{id}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Open this URL") {
			t.Fatal(buf.String())
		}
		// Restart through the API to obtain a fresh matching state for the text output path.
		start, err := skills.StartOAuthFlow(skills.OAuthStartInput{Provider: skills.OAuthProvider(id), Profile: skillsAuthProfile, ClientID: "fixture", ClientSecret: "fixture", RedirectURI: "http://localhost/callback"})
		if err != nil {
			t.Fatal(err)
		}
		skillsAuthCode = "fixture"
		skillsAuthState = start.State
		buf.Reset()
		if err := skillsAuthCompleteCmd.RunE(cmd, []string{id}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Token stored") {
			t.Fatal(buf.String())
		}
	}
	skillsAuthCallbackURL = ":invalid"
	if err := skillsAuthCompleteCmd.RunE(cmd, []string{"m365"}); err == nil {
		t.Fatal("invalid callback accepted")
	}
	skillsAuthCallbackURL = ""
	skillsAuthState = "invalid"
	if err := skillsAuthCompleteCmd.RunE(cmd, []string{"m365"}); err == nil {
		t.Fatal("invalid state accepted")
	}
	if err := skillsAuthStartCmd.RunE(cmd, []string{"unknown"}); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
