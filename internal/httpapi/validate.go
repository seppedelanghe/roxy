package httpapi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/seppedelanghe/roxy/internal/preset"
)

type Request struct {
	File      string
	Res       string
	EmbedOnly bool
	WB        string
	Exp       float64
	HalfSize  bool
	Preset    preset.Preset
}

var allowedParams = map[string]struct{}{
	"file": {}, "res": {}, "embed_only": {}, "wb": {}, "exp": {}, "half_size": {},
}

func ParseRequest(q url.Values) (Request, error) {
	for k := range q {
		if _, ok := allowedParams[k]; !ok {
			return Request{}, fmt.Errorf("%w: unknown param %q", ErrBadRequest, k)
		}
	}
	r := Request{
		File: q.Get("file"),
		Res:  defStr(q, "res", "source"),
		WB:   defStr(q, "wb", "auto"),
	}
	if r.File == "" {
		return Request{}, fmt.Errorf("%w: file is required", ErrBadRequest)
	}
	if strings.Contains(r.File, "..") || strings.HasPrefix(r.File, "/") {
		return Request{}, ErrInvalidFileKey
	}

	p, ok := preset.Lookup(r.Res)
	if !ok {
		return Request{}, fmt.Errorf("%w: invalid res %q", ErrBadRequest, r.Res)
	}
	r.Preset = p

	if r.WB != "auto" && r.WB != "camera" && r.WB != "daylight" {
		return Request{}, fmt.Errorf("%w: invalid wb", ErrBadRequest)
	}

	if v := q.Get("exp"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < -3 || f > 3 {
			return Request{}, fmt.Errorf("%w: invalid exp", ErrBadRequest)
		}
		r.Exp = f
	}

	var err error
	if r.EmbedOnly, err = parseBool(q.Get("embed_only"), false); err != nil {
		return Request{}, fmt.Errorf("%w: invalid embed_only", ErrBadRequest)
	}
	if r.HalfSize, err = parseBool(q.Get("half_size"), false); err != nil {
		return Request{}, fmt.Errorf("%w: invalid half_size", ErrBadRequest)
	}

	return r, nil
}

func defStr(q url.Values, k, def string) string {
	if v := q.Get(k); v != "" {
		return v
	}
	return def
}

func parseBool(v string, def bool) (bool, error) {
	if v == "" {
		return def, nil
	}
	return strconv.ParseBool(v)
}
