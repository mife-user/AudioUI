package valueobject

import "fmt"

// ContextHashes bind an initial prompt to its rendered context (including the
// workspace manifest), graph ancestors, and target parameters respectively.
type ContextHashes struct {
	ContextHash  string `json:"contextHash"`
	AncestorHash string `json:"ancestorHash"`
	ParamsHash   string `json:"paramsHash"`
}

// ContextEstimate describes the preflight estimate for the complete prompt.
// Actual CLI usage is recorded by the run service after execution.
type ContextEstimate struct {
	EstimatorVersion int `json:"estimatorVersion"`
	EstimatedTokens  int `json:"estimatedTokens"`
	TokenBudget      int `json:"tokenBudget"`
}

// ContextUsage combines preflight values with the actual input-token count.
// The run service is responsible for persisting it after a run completes.
type ContextUsage struct {
	ContextEstimate
	ActualInputTokens int  `json:"actualInputTokens"`
	OverBudget        bool `json:"overBudget"`
}

func (e ContextEstimate) WithActualInputTokens(actual int) (ContextUsage, error) {
	if e.EstimatorVersion <= 0 || e.EstimatedTokens < 0 || e.TokenBudget <= 0 || actual < 0 {
		return ContextUsage{}, fmt.Errorf("invalid context usage: estimator=%d estimated=%d budget=%d actual=%d", e.EstimatorVersion, e.EstimatedTokens, e.TokenBudget, actual)
	}
	return ContextUsage{ContextEstimate: e, ActualInputTokens: actual, OverBudget: actual > e.TokenBudget}, nil
}
