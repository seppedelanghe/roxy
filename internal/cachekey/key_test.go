package cachekey

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComputeDeterministic(t *testing.T) {
	in := Input{
		BackendID:   "local:/x",
		File:        "a.NEF",
		SourceSize:  123,
		SourceMTime: time.Unix(0, 1700000000000000000),
		Res:         "1080p",
		Format:      "webp",
		WB:          "auto",
		Exp:         0,
		HalfSize:    false,
		EmbedOnly:   false,
	}
	k1 := Compute(in)
	k2 := Compute(in)
	require.Equal(t, k1, k2)
	require.Len(t, k1, 64)
}

func TestDifferentFormatYieldsDifferentKey(t *testing.T) {
	in := Input{BackendID: "x", File: "a", SourceSize: 1, SourceMTime: time.Unix(0, 1), Res: "720p", Format: "jpeg", WB: "auto"}
	a := Compute(in)
	in.Format = "webp"
	b := Compute(in)
	require.NotEqual(t, a, b)
}

func TestDifferentMTimeYieldsDifferentKey(t *testing.T) {
	in := Input{BackendID: "x", File: "a", SourceSize: 1, SourceMTime: time.Unix(0, 1), Res: "720p", Format: "jpeg", WB: "auto"}
	a := Compute(in)
	in.SourceMTime = time.Unix(0, 2)
	b := Compute(in)
	require.NotEqual(t, a, b)
}
