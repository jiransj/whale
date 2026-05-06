package agent

import (
	"encoding/json"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

type FailureClass string

const (
	FailureClassTimeout        FailureClass = "timeout"
	FailureClassExecFailed     FailureClass = "exec_failed"
	FailureClassParseFailed    FailureClass = "parse_failed"
	FailureClassEmptyOutput    FailureClass = "empty_output"
	FailureClassPolicyDenied   FailureClass = "policy_denied"
	FailureClassApprovalDenied FailureClass = "approval_denied"
	FailureClassPlanRequired   FailureClass = "plan_required"
	FailureClassUnknown        FailureClass = "unknown"
)

type RecoveryAction string

const (
	RecoveryActionRetrySame        RecoveryAction = "retry_same"
	RecoveryActionRetryWithBackoff RecoveryAction = "retry_with_backoff"
	RecoveryActionFallbackReadOnly RecoveryAction = "fallback_readonly"
	RecoveryActionRequestReplan    RecoveryAction = "request_replan"
	RecoveryActionHardBlock        RecoveryAction = "hard_block"
)

type RecoveryRule struct {
	Action      RecoveryAction
	MaxAttempts int
	BackoffMS   int
}

type RecoveryPolicy struct {
	Enabled bool
	Rules   map[FailureClass]RecoveryRule
}

func DefaultRecoveryPolicy() RecoveryPolicy {
	return RecoveryPolicy{
		Enabled: true,
		Rules: map[FailureClass]RecoveryRule{
			FailureClassTimeout:        {Action: RecoveryActionRetryWithBackoff, MaxAttempts: 2, BackoffMS: 200},
			FailureClassParseFailed:    {Action: RecoveryActionRetrySame, MaxAttempts: 1},
			FailureClassEmptyOutput:    {Action: RecoveryActionRetrySame, MaxAttempts: 1},
			FailureClassExecFailed:     {Action: RecoveryActionRequestReplan, MaxAttempts: 0},
			FailureClassPolicyDenied:   {Action: RecoveryActionHardBlock, MaxAttempts: 0},
			FailureClassApprovalDenied: {Action: RecoveryActionHardBlock, MaxAttempts: 0},
			FailureClassPlanRequired:   {Action: RecoveryActionHardBlock, MaxAttempts: 0},
			FailureClassUnknown:        {Action: RecoveryActionRequestReplan, MaxAttempts: 0},
		},
	}
}

func classifyToolFailure(res core.ToolResult, dispatchErr error) FailureClass {
	if dispatchErr != nil {
		msg := strings.ToLower(dispatchErr.Error())
		if strings.Contains(msg, "timeout") {
			return FailureClassTimeout
		}
		return FailureClassUnknown
	}
	if !res.IsError {
		if strings.TrimSpace(res.Content) == "" {
			return FailureClassEmptyOutput
		}
		return ""
	}
	var env struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(res.Content), &env); err == nil {
		switch strings.TrimSpace(env.Code) {
		case "timeout":
			return FailureClassTimeout
		case "exec_failed":
			return FailureClassExecFailed
		case "parse_failed", "invalid_args", "invalid_plan_update":
			return FailureClassParseFailed
		case "policy_denied":
			return FailureClassPolicyDenied
		case "approval_denied":
			return FailureClassApprovalDenied
		case "plan_required":
			return FailureClassPlanRequired
		}
	}
	lc := strings.ToLower(res.Content)
	if strings.Contains(lc, "timeout") {
		return FailureClassTimeout
	}
	return FailureClassUnknown
}
