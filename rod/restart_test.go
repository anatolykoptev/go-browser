package rod

import (
	"errors"
	"testing"
)

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"normal error", errors.New("timeout"), false},
		{"websocket close", errors.New("websocket close 1006"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"use of closed", errors.New("use of closed network connection"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"EOF", errors.New("unexpected EOF"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConnectionError(tt.err); got != tt.want {
				t.Errorf("isConnectionError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
