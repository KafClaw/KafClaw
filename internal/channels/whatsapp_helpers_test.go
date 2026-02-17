package channels

import (
	"testing"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/config"
)

func TestWhatsAppHelperFunctions(t *testing.T) {
	if !shouldDropSystemNoise("messageContextInfo { a:b }") {
		t.Fatal("expected system noise to be dropped")
	}
	if shouldDropSystemNoise("hello") {
		t.Fatal("did not expect plain text to be dropped")
	}
	if !shouldDropReaction("reactionMessage:{key:{") {
		t.Fatal("expected reaction payload to be dropped")
	}
	if traceIDFromEvent("abc") != "wa-abc" {
		t.Fatal("expected trace id prefix")
	}
	if len(traceIDFromEvent("")) == 0 {
		t.Fatal("expected generated trace id")
	}
	if shorten("abcdef", 3) != "abc..." {
		t.Fatalf("unexpected shorten result: %q", shorten("abcdef", 3))
	}
	if !isEnglish("hello how are you today") {
		t.Fatal("expected english heuristic positive")
	}
	if isEnglish("") {
		t.Fatal("expected english heuristic false for empty text")
	}
}

func TestListParsingAndFormatting(t *testing.T) {
	list := parseList("[\"a\",\"b\",\"a\"]")
	if len(list) != 2 {
		t.Fatalf("expected deduplicated json list, got %+v", list)
	}

	list2 := parseList("a, b\n c\r\n")
	if len(list2) != 3 {
		t.Fatalf("expected parsed csv/newline list, got %+v", list2)
	}

	formatted := formatList([]string{"x", "x", "y"})
	if formatted == "" {
		t.Fatal("expected non-empty formatted list")
	}

	if !containsStr([]string{"x", "y"}, "y") {
		t.Fatal("expected containsStr true")
	}
}

func TestAuthAndPendingListHelpers(t *testing.T) {
	tl := newTestTimeline(t)
	_ = tl.SetSetting("whatsapp_allowlist", "[\"u1\",\"u2\"]")
	_ = tl.SetSetting("whatsapp_denylist", "[\"u3\"]")

	wa := NewWhatsAppChannel(config.WhatsAppConfig{AllowFrom: []string{"u4"}}, bus.NewMessageBus(), nil, tl)
	wa.loadAuthSettings()

	if !wa.isAllowed("u1") {
		t.Fatal("expected allowlist user")
	}
	if !wa.isAllowed("u4") {
		t.Fatal("expected config allow-from user")
	}
	if wa.isAllowed("u3") {
		t.Fatal("expected denylist to take precedence")
	}
	if wa.isAllowed("ux") {
		t.Fatal("expected unknown user denied")
	}

	wa.addPending("u1")
	wa.addPending("u1")
	raw, err := tl.GetSetting("whatsapp_pending")
	if err != nil {
		t.Fatalf("read pending: %v", err)
	}
	pending := parseList(raw)
	if len(pending) != 1 || pending[0] != "u1" {
		t.Fatalf("unexpected pending list: %+v", pending)
	}
}
