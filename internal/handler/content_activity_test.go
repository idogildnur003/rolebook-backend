package handler

import (
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/model"
)

func assertNoSecret(t *testing.T, d activity.Desc, secret string) {
	t.Helper()
	text := d.Title + d.Detail + d.Type
	for _, c := range d.Changes {
		text += c.Field
		if c.Before != nil || c.After != nil {
			t.Fatalf("change stores a value: %+v", c)
		}
	}
	if strings.Contains(text, secret) {
		t.Fatalf("secret leaked: %+v", d)
	}
}

var noteCampaign = &model.Campaign{Sessions: []model.Session{{ID: "s11", Name: "Session 11"}}}

func TestSessionNoteDesc_NoText(t *testing.T) {
	d := sessionNoteDesc(noteCampaign, "s11")
	if d.Detail != "Session 11" {
		t.Fatalf("got %+v", d)
	}
	assertNoSecret(t, d, "my private note")
}

func TestContentNoteDesc_NoText(t *testing.T) {
	d := contentNoteDesc("spell")
	if d.Detail != "On a spell" {
		t.Fatalf("got %+v", d)
	}
	assertNoSecret(t, d, "my private note")
}

func TestRequestDecisionDesc(t *testing.T) {
	req := &model.ContentRequest{ProposedByUserID: "u1", ItemPayload: &model.CustomEquipment{Name: "Moonlit Dagger"}}
	d := requestDecisionDesc(req)
	if d.SubjectUserID != "u1" || d.Detail != "Moonlit Dagger" {
		t.Fatalf("got %+v", d)
	}
}

func TestRequestDecisionDesc_Spell(t *testing.T) {
	req := &model.ContentRequest{ProposedByUserID: "u1", SpellPayload: &model.CustomSpell{Name: "Moonbeam II"}}
	if d := requestDecisionDesc(req); d.Detail != "Moonbeam II" {
		t.Fatalf("got %+v", d)
	}
}

func TestRequestWithdrawDesc(t *testing.T) {
	req := &model.ContentRequest{ProposedByUserID: "u1", ItemPayload: &model.CustomEquipment{Name: "Moonlit Dagger"}}
	d := requestWithdrawDesc(req)
	if d.SubjectUserID != "" || d.Detail != "Moonlit Dagger" {
		t.Fatalf("got %+v", d)
	}
}

func TestAvailabilityDesc(t *testing.T) {
	d := sessionDesc(noteCampaign, "s11")
	if d.Detail != "Session 11" {
		t.Fatalf("got %+v", d)
	}
	if d := sessionDesc(noteCampaign, "missing"); d.Detail != "" {
		t.Fatalf("missing: %+v", d)
	}
}
