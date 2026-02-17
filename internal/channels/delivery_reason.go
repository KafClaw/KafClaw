package channels

import (
	"regexp"
	"strings"
)

type deliveryClass string

const (
	deliveryTransient deliveryClass = "transient"
	deliveryTerminal  deliveryClass = "terminal"
)

func classifyDeliveryError(err error) (reasonCode string, class deliveryClass) {
	if err == nil {
		return "", deliveryTransient
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	status := extractStatusCode(msg)
	switch {
	case status == 429 || strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests"):
		return "transient:rate_limited", deliveryTransient
	case status >= 500 && status <= 599:
		return "transient:upstream_5xx", deliveryTransient
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "tempor") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset"):
		return "transient:network", deliveryTransient
	case status == 401 || status == 403:
		return "terminal:unauthorized", deliveryTerminal
	case status == 400 || status == 404 || status == 410 || status == 422:
		return "terminal:invalid_target_or_payload", deliveryTerminal
	default:
		return "terminal:send_failed", deliveryTerminal
	}
}

var statusCodeRe = regexp.MustCompile(`status[:= ]+([0-9]{3})`)

func extractStatusCode(msg string) int {
	m := statusCodeRe.FindStringSubmatch(msg)
	if len(m) < 2 {
		return 0
	}
	switch m[1] {
	case "400":
		return 400
	case "401":
		return 401
	case "403":
		return 403
	case "404":
		return 404
	case "410":
		return 410
	case "422":
		return 422
	case "429":
		return 429
	case "500":
		return 500
	case "501":
		return 501
	case "502":
		return 502
	case "503":
		return 503
	case "504":
		return 504
	default:
		return 0
	}
}
