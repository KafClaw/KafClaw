package channels

import "testing"

func TestBuildSessionScopeModes(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "default room", mode: "", want: "slack:acct-a:C1"},
		{name: "room", mode: "room", want: "slack:acct-a:C1"},
		{name: "channel", mode: "channel", want: "slack"},
		{name: "account", mode: "account", want: "slack:acct-a"},
		{name: "thread", mode: "thread", want: "slack:acct-a:C1:T1"},
		{name: "user", mode: "user", want: "slack:acct-a:U1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSessionScope("slack", "acct-a", "C1", "T1", "U1", tt.mode)
			if got != tt.want {
				t.Fatalf("scope=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestBuildSessionScopeThreadFallsBackToRoom(t *testing.T) {
	got := buildSessionScope("msteams", "default", "conv-1", "", "user-1", "thread")
	if got != "msteams:default:conv-1" {
		t.Fatalf("scope=%q", got)
	}
}

func TestBuildSessionScopeAdditionalBranches(t *testing.T) {
	if got := buildSessionScope("", "", "conv-1", "", "", "channel"); got != "channel" {
		t.Fatalf("expected default channel scope, got %q", got)
	}
	if got := buildSessionScope("slack", "acct-a", "C1", "", "", "user"); got != "slack:acct-a:C1" {
		t.Fatalf("expected user fallback to chat id, got %q", got)
	}
	if got := buildSessionScope("slack", "acct-a", "C1", "T1", "U1", "unknown"); got != "slack:acct-a:C1" {
		t.Fatalf("expected unknown mode to fallback room scope, got %q", got)
	}
}
