package httpapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNegotiate(t *testing.T) {
	cases := []struct {
		accept string
		want   string
		ok     bool
	}{
		{"", "jpeg", true},
		{"*/*", "jpeg", true},
		{"image/webp", "webp", true},
		{"image/jpeg, image/png;q=0.8", "jpeg", true},
		{"image/png;q=0.8, image/webp;q=0.9", "webp", true},
		{"image/heif", "", false},
		{"image/heif, image/avif", "", false},
		{"image/webp;q=0, image/jpeg", "jpeg", true},
	}
	for _, c := range cases {
		got, ok := Negotiate(c.accept)
		require.Equal(t, c.ok, ok, c.accept)
		require.Equal(t, c.want, got, c.accept)
	}
}
