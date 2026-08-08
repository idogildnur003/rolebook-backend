package handler

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/initiative"
	"github.com/elad/rolebook-backend/internal/model"
)

func mkParticipant(id, playerID string, init int) model.InitiativeParticipant {
	v := init
	return model.InitiativeParticipant{ID: id, PlayerID: playerID, Name: id, Initiative: &v}
}

// Rotating then re-syncing keeps every submitted participant in the cycle.
func TestEndTurnRotation_PreservesParticipants(t *testing.T) {
	c := &model.InitiativeCall{
		Status:              "open",
		HasTurnCycleStarted: true,
		Participants: []model.InitiativeParticipant{
			mkParticipant("a", "p1", 20),
			mkParticipant("b", "p2", 10),
			mkParticipant("c", "", 5),
		},
		TurnOrderParticipantIDs:  []string{"a", "b", "c"},
		CurrentTurnParticipantID: "a",
	}
	order := initiative.TurnOrderParticipantIDs(c)
	next := append(append([]string{}, order[1:]...), order[0])
	c.TurnOrderParticipantIDs = next
	c.CurrentTurnParticipantID = next[0]
	initiative.SyncTurnState(c)
	got := append([]string{}, c.TurnOrderParticipantIDs...)
	if len(got) != 3 {
		t.Fatalf("lost participants: %v", got)
	}
	if c.CurrentTurnParticipantID != got[0] {
		t.Fatalf("current %q != first %q", c.CurrentTurnParticipantID, got[0])
	}
}

// A fresh call with no rolls yields an empty order (end-turn must reject).
func TestTurnOrderEmpty_WhenNoRolls(t *testing.T) {
	c := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			{ID: "a", PlayerID: "p1", Name: "A", Initiative: nil},
		},
	}
	if got := initiative.TurnOrderParticipantIDs(c); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("expected empty order, got %v", got)
	}
}

// Participants persist the *stored* avatar value (an S3 key). Every wire
// emission must rewrite it into a presigned URL — clients render
// participant.avatarUri directly and cannot presign a bare key.
func TestWithResolvedAvatars_ResolvesParticipantAvatars(t *testing.T) {
	call := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			{ID: "player-1", PlayerID: "p1", Name: "A", AvatarURI: "players/p1/avatar/a.png"},
			{ID: "enemy-2", Name: "Goblin", AvatarURI: "players/p2/avatar/b.png"},
			{ID: "enemy-3", Name: "Ad-hoc", AvatarURI: ""},
		},
	}
	got := withResolvedAvatars(call, func(s string) string { return "https://cdn.example/" + s + "?sig=x" })

	if got.Participants[0].AvatarURI != "https://cdn.example/players/p1/avatar/a.png?sig=x" {
		t.Fatalf("player avatar not resolved: %q", got.Participants[0].AvatarURI)
	}
	if got.Participants[1].AvatarURI != "https://cdn.example/players/p2/avatar/b.png?sig=x" {
		t.Fatalf("enemy avatar not resolved: %q", got.Participants[1].AvatarURI)
	}
	if got.Participants[2].AvatarURI != "" {
		t.Fatalf("empty avatar must stay empty, got %q", got.Participants[2].AvatarURI)
	}
}

// The resolved copy is for the wire only. The stored call must keep its keys:
// it is written back to Mongo by later mutations, and the hub hands the same
// pointer to every subscriber concurrently.
func TestWithResolvedAvatars_DoesNotMutateStoredCall(t *testing.T) {
	call := &model.InitiativeCall{
		Status:       "open",
		Participants: []model.InitiativeParticipant{{ID: "player-1", PlayerID: "p1", AvatarURI: "players/p1/avatar/a.png"}},
	}
	_ = withResolvedAvatars(call, func(string) string { return "https://cdn.example/signed" })

	if call.Participants[0].AvatarURI != "players/p1/avatar/a.png" {
		t.Fatalf("stored call was mutated: %q", call.Participants[0].AvatarURI)
	}
}

// End-to-end through the real avatarstore (presigning is offline signing, so
// this needs no network, no S3 and no Mongo): a stored key must leave wireCall
// as a fetchable presigned URL, and an already-qualified URL must pass through.
func TestWireCall_PresignsStoredKeys(t *testing.T) {
	h := &InitiativeHandler{avatars: avatarstore.New(config.Config{
		AWSS3Endpoint:      "https://s3.example",
		AWSRegion:          "us-east-1",
		AWSS3Bucket:        "bucket",
		AWSAccessKeyID:     "access",
		AWSSecretAccessKey: "secret",
	})}
	call := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			{ID: "player-1", PlayerID: "p1", Name: "A", AvatarURI: "players/p1/avatar/a.png"},
			{ID: "player-2", PlayerID: "p2", Name: "B", AvatarURI: "https://cdn.example/external.png"},
		},
	}

	got := h.wireCall(context.Background(), call)

	signed := got.Participants[0].AvatarURI
	if !strings.HasPrefix(signed, "https://") {
		t.Fatalf("stored key was not presigned, client cannot render it: %q", signed)
	}
	if !strings.Contains(signed, "players/p1/avatar/a.png") {
		t.Fatalf("presigned URL lost the object path: %q", signed)
	}
	if !strings.Contains(signed, "X-Amz-Signature=") {
		t.Fatalf("presigned URL carries no signature: %q", signed)
	}
	if got.Participants[1].AvatarURI != "https://cdn.example/external.png" {
		t.Fatalf("qualified URL should pass through, got %q", got.Participants[1].AvatarURI)
	}
	if call.Participants[0].AvatarURI != "players/p1/avatar/a.png" {
		t.Fatalf("stored call must keep its key, got %q", call.Participants[0].AvatarURI)
	}
}

// An unconfigured avatarstore (local dev without S3 creds) must not panic or
// blank the field — it degrades to pass-through.
func TestWireCall_UnconfiguredStorePassesThrough(t *testing.T) {
	h := &InitiativeHandler{avatars: avatarstore.New(config.Config{})}
	call := &model.InitiativeCall{
		Participants: []model.InitiativeParticipant{{ID: "player-1", AvatarURI: "players/p1/avatar/a.png"}},
	}
	if got := h.wireCall(context.Background(), call); got.Participants[0].AvatarURI != "players/p1/avatar/a.png" {
		t.Fatalf("unconfigured store should pass through, got %q", got.Participants[0].AvatarURI)
	}
}
