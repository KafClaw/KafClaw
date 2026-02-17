package channels

import "strings"

const accountChatPrefix = "acct://"

func accountIDOrDefault(accountID string) string {
	if id := strings.TrimSpace(accountID); id != "" {
		return strings.ToLower(id)
	}
	return "default"
}

func withAccountChat(accountID, chatID string) string {
	chat := strings.TrimSpace(chatID)
	id := accountIDOrDefault(accountID)
	if id == "default" {
		return chat
	}
	return accountChatPrefix + id + "|" + chat
}

func parseAccountChat(raw string) (accountID, chatID string) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), accountChatPrefix) {
		return "default", raw
	}
	rest := strings.TrimPrefix(raw, accountChatPrefix)
	parts := strings.SplitN(rest, "|", 2)
	if len(parts) != 2 {
		return "default", raw
	}
	id := accountIDOrDefault(parts[0])
	chat := strings.TrimSpace(parts[1])
	if chat == "" {
		return "default", raw
	}
	return id, chat
}
