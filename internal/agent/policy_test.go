package agent

import "testing"

func TestParseApprovalMode(t *testing.T) {
	mode, err := ParseApprovalMode("on-request")
	if err != nil || mode != ApprovalModeOnRequest {
		t.Fatalf("unexpected parse result: mode=%s err=%v", mode, err)
	}
	mode, err = ParseApprovalMode("never")
	if err != nil || mode != ApprovalModeNever {
		t.Fatalf("unexpected parse result: mode=%s err=%v", mode, err)
	}
	mode, err = ParseApprovalMode("never-ask")
	if err != nil || mode != ApprovalModeNever {
		t.Fatalf("unexpected never-ask parse result: mode=%s err=%v", mode, err)
	}
	if _, err := ParseApprovalMode("bad"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestDefaultToolPolicyPrefixRules(t *testing.T) {
	p := DefaultToolPolicy{
		Mode:          ApprovalModeOnRequest,
		AllowPrefixes: []string{"echo"},
		DenyPrefixes:  []string{"rm -rf"},
	}
	spec := ToolSpec{Name: "shell_run"}
	allow := p.Decide(spec, ToolCall{Name: "shell_run", Input: `{"command":"echo hi"}`})
	if !allow.Allow || allow.RequiresApproval {
		t.Fatalf("expected allow-prefix decision: %+v", allow)
	}
	deny := p.Decide(spec, ToolCall{Name: "shell_run", Input: `{"command":"rm -rf /tmp/x"}`})
	if deny.Allow || deny.Code != "policy_denied" {
		t.Fatalf("expected deny-prefix decision: %+v", deny)
	}
}

func TestDefaultToolPolicyApplyPatchNeedsApproval(t *testing.T) {
	p := DefaultToolPolicy{Mode: ApprovalModeOnRequest}
	spec := ToolSpec{Name: "apply_patch"}
	d := p.Decide(spec, ToolCall{Name: "apply_patch", Input: `{"patch":"*** Begin Patch\n*** End Patch"}`})
	if !d.Allow || !d.RequiresApproval || d.Code != "approval_required" {
		t.Fatalf("unexpected apply_patch policy decision: %+v", d)
	}
}
