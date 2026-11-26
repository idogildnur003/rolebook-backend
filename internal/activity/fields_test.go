package activity

import (
	"reflect"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestDiffFields_ValuesAndLabels(t *testing.T) {
	b := model.MapPin{ID: "a", Title: "Old Mill", X: 1, Visibility: model.Visibility{SharedWithAll: false}}
	a := model.MapPin{ID: "b", Title: "New Mill", X: 2, Visibility: model.Visibility{SharedWithAll: true, SharedPlayerIds: []string{"p1"}}}
	got := DiffFields(&b, &a)
	want := []model.ActivityChange{
		{Field: "title", Before: "Old Mill", After: "New Mill"},
		{Field: "x", Before: 1.0, After: 2.0},
		{Field: "shared with all", Before: false, After: true},
		{Field: "shared players"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestDiffFields_LongTextAndImagesHaveNoValues(t *testing.T) {
	long := strings.Repeat("a", maxValueLen+1)
	got := DiffFields(model.NPC{FullDescription: "x", AvatarURI: "k1", ShortNotes: "a"},
		model.NPC{FullDescription: long, AvatarURI: "k2", ShortNotes: "line\nbreak"})
	want := []model.ActivityChange{{Field: "short notes"}, {Field: "full description"}, {Field: "avatar image"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestDiffFields_HideAndNilEmpty(t *testing.T) {
	if got := DiffFields(model.LimitedUse{Note: "a"}, model.LimitedUse{Note: "b"}, "note"); len(got) != 0 {
		t.Fatalf("hidden field logged: %+v", got)
	}
	if got := DiffFields(model.Location{LinkedNpcIds: nil}, model.Location{LinkedNpcIds: []string{}}); len(got) != 0 {
		t.Fatalf("nil vs empty list logged: %+v", got)
	}
	n := 3
	got := DiffFields(model.CustomEquipment{ArmorClass: nil}, model.CustomEquipment{ArmorClass: &n})
	want := []model.ActivityChange{{Field: "armor class", Before: nil, After: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}
