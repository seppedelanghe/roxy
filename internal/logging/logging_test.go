package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	lg := NewTo(&buf, "info")
	lg.Info("hello", "k", 1)

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Equal(t, "hello", got["msg"])
	require.Equal(t, float64(1), got["k"])
}
