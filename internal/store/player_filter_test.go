package store

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestPlayerListFilter_DefaultExcludesCompanions(t *testing.T) {
	f := playerListFilter("camp", "u1", false, "")
	if !reflect.DeepEqual(f["kind"], bson.M{"$ne": "companion"}) || f["linkedUserId"] != "u1" || f["campaignId"] != "camp" {
		t.Fatalf("got %v", f)
	}
}

func TestPlayerListFilter_DMNoOwnerFilterStillExcludesCompanions(t *testing.T) {
	f := playerListFilter("camp", "u1", true, "")
	if _, ok := f["linkedUserId"]; ok {
		t.Fatal("DM filter must not restrict linkedUserId")
	}
	if !reflect.DeepEqual(f["kind"], bson.M{"$ne": "companion"}) {
		t.Fatalf("got %v", f)
	}
}

func TestPlayerListFilter_ExplicitKindWins(t *testing.T) {
	if f := playerListFilter("camp", "u1", false, model.PlayerKindPC); f["kind"] != model.PlayerKindPC {
		t.Fatalf("got %v", f)
	}
}

func TestCompanionListFilter_PlayerSeesOnlyOwn(t *testing.T) {
	f := companionListFilter("camp", "u1", false)
	if f["kind"] != "companion" || f["linkedUserId"] != "u1" {
		t.Fatalf("got %v", f)
	}
	if _, ok := companionListFilter("camp", "u1", true)["linkedUserId"]; ok {
		t.Fatal("DM sees all companions")
	}
}
