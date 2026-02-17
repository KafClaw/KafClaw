package channels

import "fmt"

func BuildPairingReply(channel, senderLabel, code string) string {
	return fmt.Sprintf(
		"KafClaw: access not configured.\n\n%s\n\nPairing code: %s\n\nAsk the bot owner to approve with:\n`kafclaw pairing approve %s %s`",
		senderLabel,
		code,
		channel,
		code,
	)
}
