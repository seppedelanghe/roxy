package httpapi

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRequestHappyPath(t *testing.T) {
	q, _ := url.ParseQuery("file=a/b.NEF&res=720p&wb=auto&exp=0&embed_only=false&half_size=false")
	r, err := ParseRequest(q)
	require.NoError(t, err)
	require.Equal(t, "a/b.NEF", r.File)
	require.Equal(t, "720p", r.Res)
	require.Equal(t, "auto", r.WB)
	require.Equal(t, 0.0, r.Exp)
}

func TestParseRequestDefaults(t *testing.T) {
	q, _ := url.ParseQuery("file=x.NEF")
	r, err := ParseRequest(q)
	require.NoError(t, err)
	require.Equal(t, "source", r.Res)
	require.Equal(t, "auto", r.WB)
	require.Equal(t, 0.0, r.Exp)
	require.False(t, r.EmbedOnly)
	require.False(t, r.HalfSize)
}

func TestParseRequestRejectsUnknownParam(t *testing.T) {
	q, _ := url.ParseQuery("file=x.NEF&bogus=1")
	_, err := ParseRequest(q)
	require.ErrorIs(t, err, ErrBadRequest)
}

func TestParseRequestRejectsTraversal(t *testing.T) {
	q, _ := url.ParseQuery("file=../etc/passwd")
	_, err := ParseRequest(q)
	require.ErrorIs(t, err, ErrInvalidFileKey)
}

func TestParseRequestExpRange(t *testing.T) {
	q, _ := url.ParseQuery("file=x.NEF&exp=4")
	_, err := ParseRequest(q)
	require.ErrorIs(t, err, ErrBadRequest)
}

func TestParseRequestUnknownRes(t *testing.T) {
	q, _ := url.ParseQuery("file=x.NEF&res=4k")
	_, err := ParseRequest(q)
	require.ErrorIs(t, err, ErrBadRequest)
}

func TestParseRequestEmbedOnlyWithSlowPathParams(t *testing.T) {
	q, _ := url.ParseQuery("file=x.NEF&embed_only=true&wb=camera")
	_, err := ParseRequest(q)
	require.ErrorIs(t, err, ErrBadRequest)

	q, _ = url.ParseQuery("file=x.NEF&embed_only=true&exp=1.5")
	_, err = ParseRequest(q)
	require.ErrorIs(t, err, ErrBadRequest)
}
