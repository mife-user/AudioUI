package valueobject

import "testing"

func TestContextEstimateWithActualInputTokens(t *testing.T) {
	estimate := ContextEstimate{EstimatorVersion: 1, EstimatedTokens: 90, TokenBudget: 100}
	for _, tt := range []struct {
		actual int
		over   bool
	}{{0, false}, {100, false}, {101, true}} {
		usage, err := estimate.WithActualInputTokens(tt.actual)
		if err != nil || usage.ActualInputTokens != tt.actual || usage.OverBudget != tt.over || usage.ContextEstimate != estimate {
			t.Fatalf("actual=%d: usage=%+v err=%v", tt.actual, usage, err)
		}
	}
	for _, invalid := range []struct {
		estimate ContextEstimate
		actual   int
	}{
		{estimate, -1},
		{ContextEstimate{EstimatorVersion: 0, TokenBudget: 100}, 0},
		{ContextEstimate{EstimatorVersion: 1, EstimatedTokens: -1, TokenBudget: 100}, 0},
		{ContextEstimate{EstimatorVersion: 1, TokenBudget: 0}, 0},
	} {
		if _, err := invalid.estimate.WithActualInputTokens(invalid.actual); err == nil {
			t.Fatalf("invalid usage accepted: %+v", invalid)
		}
	}
}
