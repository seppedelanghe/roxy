package cachekey

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

type Input struct {
	BackendID   string
	File        string
	SourceSize  int64
	SourceMTime time.Time
	Res         string
	Format      string
	WB          string
	Exp         float64
	HalfSize    bool
	EmbedOnly   bool
}

func Compute(in Input) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%d|%s|%s|%s|%s|%t|%t",
		in.BackendID,
		in.File,
		in.SourceSize,
		in.SourceMTime.UnixNano(),
		in.Res,
		in.Format,
		in.WB,
		strconv.FormatFloat(in.Exp, 'f', 4, 64),
		in.HalfSize,
		in.EmbedOnly,
	)
	return hex.EncodeToString(h.Sum(nil))
}
