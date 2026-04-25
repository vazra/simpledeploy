package audit

import (
	"context"
	"testing"
)

func TestCtxRoundTrip(t *testing.T) {
	uid := int64(42)
	c := Ctx{ActorUserID: &uid, ActorName: "ameen", ActorSource: "ui", IP: "1.2.3.4"}
	ctx := With(context.Background(), c)
	got := From(ctx)
	if got.ActorName != "ameen" || got.ActorSource != "ui" || got.IP != "1.2.3.4" {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.ActorUserID == nil || *got.ActorUserID != 42 {
		t.Errorf("ActorUserID lost: %v", got.ActorUserID)
	}
}

func TestFromMissing(t *testing.T) {
	got := From(context.Background())
	if got.ActorSource != "system" {
		t.Errorf("expected default system, got %q", got.ActorSource)
	}
}
