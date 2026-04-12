package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/elad/rolebook-backend/internal/model"
)

//go:embed equipment.json spells.json
var catalogFS embed.FS

// ArsenalCatalog holds the full in-memory equipment and spell catalogs.
type ArsenalCatalog struct {
	equipment    []model.Equipment
	equipmentMap map[string]*model.Equipment
	spells       []model.Spell
	spellMap     map[string]*model.Spell
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

	return c, nil
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
