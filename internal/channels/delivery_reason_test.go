package channels

import (
	"errors"
	"testing"
)

func TestClassifyDeliveryError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  string
		wantClass deliveryClass
	}{
		{name: "rate limited", err: errors.New("status: 429"), wantCode: "transient:rate_limited", wantClass: deliveryTransient},
		{name: "server error", err: errors.New("status=503"), wantCode: "transient:upstream_5xx", wantClass: deliveryTransient},
		{name: "network timeout", err: errors.New("i/o timeout"), wantCode: "transient:network", wantClass: deliveryTransient},
		{name: "unauthorized", err: errors.New("status: 401"), wantCode: "terminal:unauthorized", wantClass: deliveryTerminal},
		{name: "bad payload", err: errors.New("status: 400"), wantCode: "terminal:invalid_target_or_payload", wantClass: deliveryTerminal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, cls := classifyDeliveryError(tt.err)
			if code != tt.wantCode || cls != tt.wantClass {
				t.Fatalf("got (%s,%s), want (%s,%s)", code, cls, tt.wantCode, tt.wantClass)
			}
		})
	}
}
