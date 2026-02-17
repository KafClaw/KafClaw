package channels

import "testing"

func TestAccountRoutingHelpers(t *testing.T) {
	if got := accountIDOrDefault(""); got != "default" {
		t.Fatalf("default account mismatch: %q", got)
	}
	if got := accountIDOrDefault(" Acct-A "); got != "acct-a" {
		t.Fatalf("normalized account mismatch: %q", got)
	}

	if got := withAccountChat("", "C1"); got != "C1" {
		t.Fatalf("default account should keep chat id, got %q", got)
	}
	if got := withAccountChat("acct-a", " C1 "); got != "acct://acct-a|C1" {
		t.Fatalf("account chat key mismatch: %q", got)
	}

	accountID, chatID := parseAccountChat("acct://acct-a|C1")
	if accountID != "acct-a" || chatID != "C1" {
		t.Fatalf("parse account chat mismatch: account=%q chat=%q", accountID, chatID)
	}

	accountID, chatID = parseAccountChat("acct://acct-a|")
	if accountID != "default" || chatID != "acct://acct-a|" {
		t.Fatalf("expected fallback for empty chat, got account=%q chat=%q", accountID, chatID)
	}

	accountID, chatID = parseAccountChat("acct://acct-a")
	if accountID != "default" || chatID != "acct://acct-a" {
		t.Fatalf("expected fallback when separator missing, got account=%q chat=%q", accountID, chatID)
	}

	accountID, chatID = parseAccountChat("C1")
	if accountID != "default" || chatID != "C1" {
		t.Fatalf("plain chat parse mismatch: account=%q chat=%q", accountID, chatID)
	}
}
