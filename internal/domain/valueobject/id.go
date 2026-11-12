package valueobject

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	ProjectIDPrefix = "p_"
	NodeIDPrefix    = "n_"
	EdgeIDPrefix    = "e_"
	RunIDPrefix     = "r_"
	PendingIDPrefix = "pa_"

	MaxIDLength = 64
)

var (
	ErrInvalidID = errors.New("INVALID_ID")

	idPattern   = regexp.MustCompile(`^[a-z0-9_]+$`)
	reservedIDs = map[string]struct{}{
		"con": {}, "prn": {}, "aux": {}, "nul": {},
		"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {}, "com6": {}, "com7": {}, "com8": {}, "com9": {},
		"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {}, "lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
	}
)

// NewID returns a program-generated identifier with the supplied prefix.
// Generated characters are lower-case hex so the result satisfies ValidateID.
func NewID(prefix string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("%w: missing prefix", ErrInvalidID)
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	id := prefix + hex.EncodeToString(buf)
	if err := ValidateID(id); err != nil {
		return "", err
	}
	return id, nil
}

// ValidateID enforces the node/project/run/pending identifier contract:
// lower-case ASCII, digits and underscores; no separators, drives, dot names
// or Windows reserved device names.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty", ErrInvalidID)
	}
	if len(id) > MaxIDLength {
		return fmt.Errorf("%w: too long", ErrInvalidID)
	}
	if id == "." || id == ".." {
		return fmt.Errorf("%w: dot name", ErrInvalidID)
	}
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	if _, ok := reservedIDs[id]; ok {
		return fmt.Errorf("%w: reserved name %q", ErrInvalidID, id)
	}
	trimmed := strings.TrimRight(id, ". ")
	if trimmed != id {
		return fmt.Errorf("%w: trailing dot or space", ErrInvalidID)
	}
	return nil
}
