package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash/fnv"
)

// FormKeyForModel encodes a model id for use in HTML form field names (safe characters).
func FormKeyForModel(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// SafeHTMLID returns a deterministic, HTML/CSS-safe id token for a model name (may contain ':' etc.).
func SafeHTMLID(modelID string) string {
	h := sha256.Sum256([]byte(modelID))
	return fmt.Sprintf("m%x", h[:8])
}

// ColorForModel returns a hex color for known cloud slots, else a stable HSL from hashing the id.
func ColorForModel(id ModelID) string {
	if c, ok := ModelColors[id]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(id))
	hue := h.Sum32() % 360
	return fmt.Sprintf("hsl(%d, 62%%, 52%%)", hue)
}
