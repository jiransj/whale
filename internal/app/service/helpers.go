package service

import "github.com/usewhale/whale/internal/policy"

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func approvalModeDisplay(mode policy.ApprovalMode) string {
	switch mode {
	case policy.ApprovalModeNever:
		return "auto approve"
	default:
		return "ask first"
	}
}
