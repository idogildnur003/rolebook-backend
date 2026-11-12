package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/elad/rolebook-backend/internal/model"
)

//go:embed equipment.json spells.json feats.json class_features/*.json
var catalogFS embed.FS

// ArsenalCatalog holds the full in-memory equipment and spell catalogs.
type ArsenalCatalog struct {
	equipment       []model.Equipment
	equipmentMap    map[string]*model.Equipment
	spells          []model.Spell
	spellMap        map[string]*model.Spell
	feats           []model.Feat
	featMap         map[string]*model.Feat
	classFeatures   []model.ClassFeature
	classFeatureMap map[string]*model.ClassFeature
}

// Load reads the embedded JSON files and returns a ready-to-use ArsenalCatalog.
func Load() (*ArsenalCatalog, error) {
	c := &ArsenalCatalog{}

	eqData, err := catalogFS.ReadFile("equipment.json")
	if err != nil {
		return nil, fmt.Errorf("read equipment.json: %w", err)
	}
	if err := json.Unmarshal(eqData, &c.equipment); err != nil {
		return nil, fmt.Errorf("parse equipment.json: %w", err)
	}
	sort.Slice(c.equipment, func(i, j int) bool {
		return c.equipment[i].Name < c.equipment[j].Name
	})
	c.equipmentMap = make(map[string]*model.Equipment, len(c.equipment))
	for i := range c.equipment {
		c.equipmentMap[c.equipment[i].ID] = &c.equipment[i]
	}

	spData, err := catalogFS.ReadFile("spells.json")
	if err != nil {
		return nil, fmt.Errorf("read spells.json: %w", err)
	}
	if err := json.Unmarshal(spData, &c.spells); err != nil {
		return nil, fmt.Errorf("parse spells.json: %w", err)
	}
	sort.Slice(c.spells, func(i, j int) bool {
		return c.spells[i].Name < c.spells[j].Name
	})
	c.spellMap = make(map[string]*model.Spell, len(c.spells))
	for i := range c.spells {
		c.spellMap[c.spells[i].ID] = &c.spells[i]
	}

	if err := c.loadFeats(); err != nil {
		return nil, err
	}
	if err := c.loadClassFeatures(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *ArsenalCatalog) loadFeats() error {
	data, err := catalogFS.ReadFile("feats.json")
	if err != nil {
		return fmt.Errorf("read feats.json: %w", err)
	}
	if err := json.Unmarshal(data, &c.feats); err != nil {
		return fmt.Errorf("parse feats.json: %w", err)
	}
	sort.Slice(c.feats, func(i, j int) bool {
		return c.feats[i].Name < c.feats[j].Name
	})
	c.featMap = make(map[string]*model.Feat, len(c.feats))
	for i := range c.feats {
		id := c.feats[i].ID
		if _, dup := c.featMap[id]; dup {
			return fmt.Errorf("feats.json: duplicate id %q", id)
		}
		c.featMap[id] = &c.feats[i]
	}
	return nil
}

// loadClassFeatures reads every class_features/<class>.json file. Features are
// ordered by class, then base class before subclasses, then subclass, level
// and name.
func (c *ArsenalCatalog) loadClassFeatures() error {
	files, err := fs.Glob(catalogFS, "class_features/*.json")
	if err != nil {
		return fmt.Errorf("list class_features: %w", err)
	}
	for _, name := range files {
		data, err := catalogFS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		var part []model.ClassFeature
		if err := json.Unmarshal(data, &part); err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		c.classFeatures = append(c.classFeatures, part...)
	}
	sort.SliceStable(c.classFeatures, func(i, j int) bool {
		a, b := c.classFeatures[i], c.classFeatures[j]
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		if a.Subclass != b.Subclass {
			return a.Subclass < b.Subclass // "" (base class) sorts first
		}
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return a.Name < b.Name
	})
	c.classFeatureMap = make(map[string]*model.ClassFeature, len(c.classFeatures))
	for i := range c.classFeatures {
		id := c.classFeatures[i].ID
		if _, dup := c.classFeatureMap[id]; dup {
			return fmt.Errorf("class_features: duplicate id %q", id)
		}
		c.classFeatureMap[id] = &c.classFeatures[i]
	}
	return nil
}

func paginate[T any](items []T, page, limit int64) ([]T, int64) {
	total := int64(len(items))
	skip := (page - 1) * limit
	if skip >= total {
		return []T{}, total
	}
	end := skip + limit
	if end > total {
		end = total
	}
	return items[skip:end], total
}

// ListFeats returns a paginated slice of feats. A non-empty category
// (origin, general, fighting-style, epic-boon) narrows the list.
func (c *ArsenalCatalog) ListFeats(category string, page, limit int64) ([]model.Feat, int64) {
	items := c.feats
	if category != "" {
		items = make([]model.Feat, 0, len(c.feats))
		for _, f := range c.feats {
			if strings.EqualFold(f.Category, category) {
				items = append(items, f)
			}
		}
	}
	return paginate(items, page, limit)
}

// GetFeat returns a single feat by ID, or nil if not found.
func (c *ArsenalCatalog) GetFeat(id string) *model.Feat {
	return c.featMap[id]
}

// ListClassFeatures returns a paginated slice of class features. A non-empty
// class narrows the list to that class (base class and every subclass). A
// non-empty subclass narrows it further to the base class plus that subclass.
// Matching ignores case.
func (c *ArsenalCatalog) ListClassFeatures(class, subclass string, page, limit int64) ([]model.ClassFeature, int64) {
	items := c.classFeatures
	if class != "" || subclass != "" {
		items = make([]model.ClassFeature, 0, len(c.classFeatures))
		for _, f := range c.classFeatures {
			if class != "" && !strings.EqualFold(f.Class, class) {
				continue
			}
			if subclass != "" && f.Subclass != "" && !strings.EqualFold(f.Subclass, subclass) {
				continue
			}
			items = append(items, f)
		}
	}
	return paginate(items, page, limit)
}

// GetClassFeature returns a single class feature by ID, or nil if not found.
func (c *ArsenalCatalog) GetClassFeature(id string) *model.ClassFeature {
	return c.classFeatureMap[id]
}

// ListEquipment returns a paginated slice of equipment.
func (c *ArsenalCatalog) ListEquipment(page, limit int64) ([]model.Equipment, int64) {
	total := int64(len(c.equipment))
	skip := (page - 1) * limit
	if skip >= total {
		return []model.Equipment{}, total
	}
	end := skip + limit
	if end > total {
		end = total
	}
	return c.equipment[skip:end], total
}

// GetEquipment returns a single equipment item by ID, or nil if not found.
func (c *ArsenalCatalog) GetEquipment(id string) *model.Equipment {
	return c.equipmentMap[id]
}

// ListSpells returns a paginated slice of spells.
func (c *ArsenalCatalog) ListSpells(page, limit int64) ([]model.Spell, int64) {
	total := int64(len(c.spells))
	skip := (page - 1) * limit
	if skip >= total {
		return []model.Spell{}, total
	}
	end := skip + limit
	if end > total {
		end = total
	}
	return c.spells[skip:end], total
}

// GetSpell returns a single spell by ID, or nil if not found.
func (c *ArsenalCatalog) GetSpell(id string) *model.Spell {
	return c.spellMap[id]
}
