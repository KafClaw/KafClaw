package channels

import "strings"

func buildSessionScope(channel, accountID, chatID, threadID, senderID, mode string) string {
	ch := strings.TrimSpace(strings.ToLower(channel))
	if ch == "" {
		ch = "channel"
	}
	account := accountIDOrDefault(accountID)
	chat := strings.TrimSpace(chatID)
	thread := strings.TrimSpace(threadID)
	sender := strings.TrimSpace(senderID)
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "channel":
		return ch
	case "account":
		return ch + ":" + account
	case "user":
		if sender == "" {
			sender = chat
		}
		return ch + ":" + account + ":" + sender
	case "thread":
		if thread == "" {
			return ch + ":" + account + ":" + chat
		}
		return ch + ":" + account + ":" + chat + ":" + thread
	default: // room
		return ch + ":" + account + ":" + chat
	}
}
