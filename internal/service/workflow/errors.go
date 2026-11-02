package workflow

import (
	"errors"

	"audio-ui/internal/domain/port"
)

var (
	ErrNodeNotFound             = errors.New("NODE_NOT_FOUND")
	ErrNodeLocked               = errors.New("NODE_LOCKED")
	ErrRunningChainLocked       = errors.New("RUNNING_CHAIN_LOCKED")
	ErrReparentConfirmRequired  = errors.New("REPARENT_CONFIRM_REQUIRED")
	ErrSelfConnect              = errors.New("SELF_CONNECT")
	ErrCycleDetected            = errors.New("CYCLE_DETECTED")
	ErrTopicRootParent          = errors.New("TOPIC_ROOT_PARENT")
	ErrNotInChain               = errors.New("NOT_IN_CHAIN")
	ErrCandidateInvalid         = errors.New("CANDIDATE_INVALID")
	ErrUnknownNodeType          = errors.New("UNKNOWN_NODE_TYPE")
	ErrWorkspaceMoveFailed      = errors.New("WORKSPACE_MOVE_FAILED")
	ErrInvalidPosition          = errors.New("INVALID_POSITION")
	ErrInvalidInput             = errors.New("INVALID_INPUT")
	ErrPendingNotFound          = port.ErrPendingNotFound
	ErrPendingAlreadyApplied    = errors.New("PENDING_ALREADY_APPLIED")
	ErrPendingNotApplicable     = errors.New("PENDING_NOT_APPLICABLE")
	ErrPendingInvalid           = errors.New("PENDING_INVALID")
	ErrTurnPendingExists        = errors.New("TURN_PENDING_EXISTS")
	ErrPendingLimit             = errors.New("PENDING_LIMIT_REACHED")
	ErrPendingPayloadTooLarge   = errors.New("PENDING_PAYLOAD_TOO_LARGE")
	ErrRequestIDConflict        = errors.New("REQUEST_ID_CONFLICT")
	ErrParamsChanged            = errors.New("PARAMS_CHANGED")
	ErrContextChanged           = errors.New("CONTEXT_CHANGED")
	ErrNodeKindMismatch         = errors.New("NODE_KIND_MISMATCH")
	ErrUnsupportedUntilM3       = errors.New("UNSUPPORTED_UNTIL_M3")
	ErrPendingStoreUnavailable  = errors.New("PENDING_STORE_UNAVAILABLE")
	ErrArtifactStoreUnavailable = errors.New("ARTIFACT_STORE_UNAVAILABLE")
	ErrContextHasherUnavailable = errors.New("CONTEXT_HASHER_UNAVAILABLE")
)
