package httpapi

import (
	"strconv"
	"strings"
)

var supported = map[string]string{
	"image/jpeg": "jpeg",
	"image/png":  "png",
	"image/webp": "webp",
}

// Negotiate parses an Accept header and returns ("jpeg"|"png"|"webp", true)
// or ("", false) if no supported type is acceptable. Empty or */* defaults to jpeg.
func Negotiate(accept string) (string, bool) {
	if accept == "" {
		return "jpeg", true
	}
	bestQ := -1.0
	best := ""
	wildcard := false
	for _, raw := range strings.Split(accept, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, ";")
		mediaType := strings.TrimSpace(parts[0])
		q := 1.0
		for _, p := range parts[1:] {
			p = strings.TrimSpace(p)
			if v, ok := strings.CutPrefix(p, "q="); ok {
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					q = parsed
				}
			}
		}
		if q <= 0 {
			continue
		}
		if mediaType == "*/*" {
			wildcard = true
			continue
		}
		fmt, ok := supported[mediaType]
		if !ok {
			continue
		}
		if q > bestQ {
			bestQ = q
			best = fmt
		}
	}
	if best != "" {
		return best, true
	}
	if wildcard {
		return "jpeg", true
	}
	return "", false
}
