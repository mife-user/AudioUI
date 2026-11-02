package persistence

import "errors"

var (
	ErrCorruptConfig      = errors.New("CONFIG_CORRUPT")
	ErrInvalidConfig      = errors.New("CONFIG_INVALID")
	ErrCorruptProject     = errors.New("PROJECT_CORRUPT")
	ErrInvalidProject     = errors.New("PROJECT_INVALID")
	ErrProjectNotFound    = errors.New("PROJECT_NOT_FOUND")
	ErrProjectExists      = errors.New("PROJECT_EXISTS")
	ErrUnsupportedVersion = errors.New("UNSUPPORTED_VERSION")
	ErrCorruptRuns        = errors.New("RUNS_CORRUPT")
	ErrInvalidRun         = errors.New("RUN_INVALID")
	ErrRunNotFound        = errors.New("RUN_NOT_FOUND")
	ErrCorruptPending     = errors.New("PENDING_CORRUPT")
	ErrInvalidPending     = errors.New("PENDING_INVALID")
	ErrPendingNotFound    = errors.New("PENDING_NOT_FOUND")
	ErrPendingTooLarge    = errors.New("PENDING_PAYLOAD_TOO_LARGE")
	ErrArtifactNotFound   = errors.New("ARTIFACT_NOT_FOUND")
	ErrArtifactCorrupt    = errors.New("ARTIFACT_CORRUPT")
	ErrArtifactInvalid    = errors.New("ARTIFACT_INVALID")
)
