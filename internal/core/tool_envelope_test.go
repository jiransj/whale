package core

import "testing"

func TestToolEnvelopeRoundTrip(t *testing.T) {
	content, err := MarshalToolEnvelope(NewToolSuccessEnvelope(map[string]any{
		"payload": map[string]any{"stdout": "ok"},
	}))
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	got, ok := ParseToolEnvelope(content)
	if !ok {
		t.Fatal("expected envelope to parse")
	}
	if !got.Success || got.Code != "ok" {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	payload, _ := got.Data["payload"].(map[string]any)
	if payload["stdout"] != "ok" {
		t.Fatalf("unexpected payload: %+v", got.Data)
	}
}

func TestToolEnvelopeParsesLegacyMessageAsError(t *testing.T) {
	got, ok := ParseToolEnvelope(`{"success":false,"code":"failed","message":"boom"}`)
	if !ok {
		t.Fatal("expected legacy envelope to parse")
	}
	if got.Error != "boom" || got.Message != "boom" {
		t.Fatalf("expected message copied to error, got %+v", got)
	}
}
