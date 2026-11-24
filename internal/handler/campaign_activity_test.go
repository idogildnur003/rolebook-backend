package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestCampaignDeleteDesc(t *testing.T) {
	d := campaignDeleteDesc(&model.Campaign{ID: "c1", Name: "Curse of the Moon"})
	if d.CampaignID != "" || d.Detail != "Curse of the Moon" {
		t.Fatalf("got %+v", d)
	}
}

func TestCampaignCreateDesc(t *testing.T) {
	d := campaignCreateDesc(&model.Campaign{ID: "c1", Name: "Curse of the Moon"})
	if d.CampaignID != "c1" || d.Detail != "Curse of the Moon" {
		t.Fatalf("got %+v", d)
	}
}

func TestPlayerActiveDesc(t *testing.T) {
	if d := playerActiveDesc(false); d.Title != "Player archived" {
		t.Fatalf("got %+v", d)
	}
	if d := playerActiveDesc(true); d.Title != "Player restored" {
		t.Fatalf("got %+v", d)
	}
}
