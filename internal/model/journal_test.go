package model

import "testing"

func TestIsValidMapPinType(t *testing.T) {
	valid := []MapPinType{MapPinLocation, MapPinNPC, MapPinItem, MapPinMajorFinding, MapPinTravelMarker, MapPinCustom}
	for _, v := range valid {
		if !IsValidMapPinType(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	if IsValidMapPinType(MapPinType("bogus")) {
		t.Error("expected bogus to be invalid")
	}
}

func TestMapPinTypeReferencesEntity(t *testing.T) {
	if !MapPinTypeReferencesEntity(MapPinLocation) {
		t.Error("location must reference entity")
	}
	if !MapPinTypeReferencesEntity(MapPinNPC) {
		t.Error("npc must reference entity")
	}
	if MapPinTypeReferencesEntity(MapPinItem) {
		t.Error("item must NOT reference entity")
	}
	if MapPinTypeReferencesEntity(MapPinMajorFinding) {
		t.Error("majorFinding must NOT reference entity")
	}
	if MapPinTypeReferencesEntity(MapPinTravelMarker) {
		t.Error("travelMarker must NOT reference entity")
	}
	if MapPinTypeReferencesEntity(MapPinCustom) {
		t.Error("custom must NOT reference entity")
	}
}
