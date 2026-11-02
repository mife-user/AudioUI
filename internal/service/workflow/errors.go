package workflow

import "errors"

var (
	ErrNodeNotFound            = errors.New("NODE_NOT_FOUND")
	ErrNodeLocked              = errors.New("NODE_LOCKED")
	ErrRunningChainLocked      = errors.New("RUNNING_CHAIN_LOCKED")
	ErrReparentConfirmRequired = errors.New("REPARENT_CONFIRM_REQUIRED")
	ErrSelfConnect             = errors.New("SELF_CONNECT")
	ErrCycleDetected           = errors.New("CYCLE_DETECTED")
	ErrTopicRootParent         = errors.New("TOPIC_ROOT_PARENT")
	ErrNotInChain              = errors.New("NOT_IN_CHAIN")
	ErrCandidateInvalid        = errors.New("CANDIDATE_INVALID")
	ErrUnknownNodeType         = errors.New("UNKNOWN_NODE_TYPE")
	ErrWorkspaceMoveFailed     = errors.New("WORKSPACE_MOVE_FAILED")
	ErrInvalidPosition         = errors.New("INVALID_POSITION")
	ErrInvalidInput            = errors.New("INVALID_INPUT")
)
