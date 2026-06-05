package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/catalog"
	"github.com/elad/rolebook-backend/internal/model"
)

// fakeCatalogImages is an in-memory catalogImageRepo for handler tests.
type fakeCatalogImages struct {
	keys map[string]string // key: "<type>:<itemId>"
}

func (f *fakeCatalogImages) KeysByType(_ context.Context, t model.CatalogImageType) (map[string]string, error) {
	out := map[string]string{}
	prefix := string(t) + ":"
	for k, v := range f.keys {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			out[k[len(prefix):]] = v
		}
	}
	return out, nil
}
func (f *fakeCatalogImages) ImageKey(_ context.Context, t model.CatalogImageType, itemID string) (string, error) {
	return f.keys[string(t)+":"+itemID], nil
}
func (f *fakeCatalogImages) SetImage(_ context.Context, t model.CatalogImageType, itemID, key string) (string, error) {
	id := string(t) + ":" + itemID
	prev := f.keys[id]
	if f.keys == nil {
		f.keys = map[string]string{}
	}
	f.keys[id] = key
	return prev, nil
}
func (f *fakeCatalogImages) DeleteImage(_ context.Context, t model.CatalogImageType, itemID string) (string, error) {
	id := string(t) + ":" + itemID
	prev := f.keys[id]
	delete(f.keys, id)
	return prev, nil
}

func newArsenalHandlerForTest(t *testing.T, images catalogImageRepo) (*ArsenalHandler, *catalog.ArsenalCatalog) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog load: %v", err)
	}
	// Unconfigured avatar store → ResolveImageURICached passes the key through.
	avatars := avatarstore.New(config.Config{})
	return NewArsenalHandler(cat, images, avatars), cat
}

func TestListEquipment_OverlaysImageKey(t *testing.T) {
	images := &fakeCatalogImages{keys: map[string]string{}}
	h, cat := newArsenalHandlerForTest(t, images)

	// Grab a real equipment ID from the catalog and attach an override.
	first, _ := cat.ListEquipment(1, 1)
	if len(first) == 0 {
		t.Fatal("catalog has no equipment")
	}
	id := first[0].ID
	images.keys["equipment:"+id] = "arsenal/equipment/" + id + "/x.png"

	req := httptest.NewRequest(http.MethodGet, "/api/arsenal/equipment?limit=100", nil)
	rr := httptest.NewRecorder()
	h.ListEquipment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Data []model.Equipment `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, e := range body.Data {
		if e.ID == id {
			found = true
			if e.ImageURI != "arsenal/equipment/"+id+"/x.png" {
				t.Fatalf("ImageURI = %q, want overlaid key (pass-through)", e.ImageURI)
			}
		}
	}
	if !found {
		t.Fatalf("equipment %q not on first page; rerun with its page", id)
	}
}

func TestGetSpell_OverlaysImageKey(t *testing.T) {
	images := &fakeCatalogImages{keys: map[string]string{}}
	h, cat := newArsenalHandlerForTest(t, images)

	first, _ := cat.ListSpells(1, 1)
	if len(first) == 0 {
		t.Fatal("catalog has no spells")
	}
	id := first[0].ID
	images.keys["spell:"+id] = "arsenal/spells/" + id + "/y.png"

	req := httptest.NewRequest(http.MethodGet, "/api/arsenal/spells/"+id, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("spellId", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.GetSpell(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var spell model.Spell
	if err := json.Unmarshal(rr.Body.Bytes(), &spell); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if spell.ImageURI != "arsenal/spells/"+id+"/y.png" {
		t.Fatalf("ImageURI = %q, want overlaid key", spell.ImageURI)
	}
}

func TestSetEquipmentImage_UnknownItem404(t *testing.T) {
	images := &fakeCatalogImages{keys: map[string]string{}}
	h, _ := newArsenalHandlerForTest(t, images)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/arsenal/equipment/nope/image",
		stringReader(`{"key":"arsenal/equipment/nope/x.png"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("equipmentId", "does-not-exist")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.SetEquipmentImage(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestSetSpellImage_PersistsAndReturnsURL(t *testing.T) {
	images := &fakeCatalogImages{keys: map[string]string{}}
	h, cat := newArsenalHandlerForTest(t, images)
	first, _ := cat.ListSpells(1, 1)
	id := first[0].ID
	key := "arsenal/spells/" + id + "/z.png"

	req := httptest.NewRequest(http.MethodPut, "/api/admin/arsenal/spells/"+id+"/image",
		stringReader(`{"key":"`+key+`"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("spellId", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.SetSpellImage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rr.Code, rr.Body.String())
	}
	if images.keys["spell:"+id] != key {
		t.Fatalf("key not persisted: %v", images.keys)
	}
}

func TestDeleteEquipmentImage_NoContent(t *testing.T) {
	images := &fakeCatalogImages{keys: map[string]string{"equipment:longsword": "k.png"}}
	h, _ := newArsenalHandlerForTest(t, images)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/arsenal/equipment/longsword/image", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("equipmentId", "longsword")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.DeleteEquipmentImage(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if _, ok := images.keys["equipment:longsword"]; ok {
		t.Fatalf("override not deleted")
	}
}

func TestSetEquipmentImage_WrongPrefix400(t *testing.T) {
	images := &fakeCatalogImages{keys: map[string]string{}}
	h, cat := newArsenalHandlerForTest(t, images)
	first, _ := cat.ListEquipment(1, 1)
	id := first[0].ID

	req := httptest.NewRequest(http.MethodPut, "/api/admin/arsenal/equipment/"+id+"/image",
		stringReader(`{"key":"players/p1/avatar/x.png"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("equipmentId", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.SetEquipmentImage(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rr.Code, rr.Body.String())
	}
}

func stringReader(s string) *strings.Reader { return strings.NewReader(s) }
