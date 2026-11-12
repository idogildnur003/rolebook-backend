package catalog

import "testing"

func TestCatalog_SpellsEnriched(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fb := c.GetSpell("spell-fireball")
	if fb == nil {
		t.Fatal("spell-fireball not found")
	}
	if fb.Damage != "8d6" || fb.DamageType != "fire" || fb.SavingThrowAbility != "DEX" {
		t.Errorf("fireball not enriched: %+v", fb)
	}
	hasWizard := false
	for _, cl := range fb.Classes {
		if cl == "Wizard" {
			hasWizard = true
		}
	}
	if !hasWizard {
		t.Errorf("fireball classes missing Wizard: %v", fb.Classes)
	}
	hm := c.GetSpell("spell-hunters-mark")
	if hm == nil || !hm.Concentration {
		t.Errorf("hunters-mark concentration not enriched: %+v", hm)
	}
}

func TestCatalog_Feats(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	all, total := c.ListFeats("", 1, 100)
	if total != 75 || len(all) != 75 {
		t.Fatalf("feats total = %d (page %d), want 75", total, len(all))
	}
	wantByCategory := map[string]int64{"origin": 10, "general": 43, "fighting-style": 10, "epic-boon": 12}
	for cat, want := range wantByCategory {
		if _, got := c.ListFeats(cat, 1, 100); got != want {
			t.Errorf("category %q: %d feats, want %d", cat, got, want)
		}
	}
	for i, f := range all {
		if f.ID == "" || f.Name == "" || len(f.Benefits) == 0 {
			t.Errorf("feat %d incomplete: %+v", i, f)
		}
		if _, ok := wantByCategory[f.Category]; !ok {
			t.Errorf("feat %q has unknown category %q", f.ID, f.Category)
		}
		if f.Category != "origin" && f.Prerequisite == "" {
			t.Errorf("feat %q should have a prerequisite", f.ID)
		}
		for _, b := range f.Benefits {
			if b.Name == "" || b.Description == "" {
				t.Errorf("feat %q has an empty benefit", f.ID)
			}
		}
		if i > 0 && all[i-1].Name > f.Name {
			t.Errorf("feats not sorted by name at %q", f.Name)
		}
	}
	alert := c.GetFeat("feat-alert")
	if alert == nil || alert.Category != "origin" || alert.Repeatable {
		t.Errorf("feat-alert wrong: %+v", alert)
	}
	if mi := c.GetFeat("feat-magic-initiate"); mi == nil || !mi.Repeatable {
		t.Errorf("feat-magic-initiate should be repeatable: %+v", mi)
	}
	if c.GetFeat("nope") != nil {
		t.Error("unknown feat should be nil")
	}
}

func TestCatalog_ClassFeatures(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	classes := []string{"Barbarian", "Bard", "Cleric", "Druid", "Fighter", "Monk",
		"Paladin", "Ranger", "Rogue", "Sorcerer", "Warlock", "Wizard"}
	_, total := c.ListClassFeatures("", "", 1, 1)
	var sum int64
	for _, class := range classes {
		// Page through the whole class.
		var feats []string
		subclasses := map[string]bool{}
		hasSubclassPick, hasEpicBoon := false, false
		for page := int64(1); ; page++ {
			data, n := c.ListClassFeatures(class, "", page, 100)
			if page == 1 {
				sum += n
			}
			if len(data) == 0 {
				break
			}
			for _, f := range data {
				feats = append(feats, f.ID)
				if f.Class != class {
					t.Errorf("%s: filter leaked %q of class %q", class, f.ID, f.Class)
				}
				if f.Level < 1 || f.Level > 20 || f.Name == "" || f.Description == "" {
					t.Errorf("%s: feature incomplete: %+v", class, f)
				}
				if f.Subclass != "" {
					subclasses[f.Subclass] = true
					if f.Level < 3 {
						t.Errorf("%s: subclass feature %q below level 3", class, f.ID)
					}
				} else {
					if f.Name == class+" Subclass" {
						hasSubclassPick = true
					}
					if f.Name == "Epic Boon" && f.Level == 19 {
						hasEpicBoon = true
					}
				}
			}
		}
		if len(subclasses) != 4 {
			t.Errorf("%s: %d subclasses, want 4: %v", class, len(subclasses), subclasses)
		}
		if !hasSubclassPick || !hasEpicBoon {
			t.Errorf("%s: missing subclass pick (%v) or level 19 Epic Boon (%v)", class, hasSubclassPick, hasEpicBoon)
		}
		if len(feats) < 20 {
			t.Errorf("%s: only %d features", class, len(feats))
		}
	}
	if sum != total {
		t.Errorf("features across the 12 classes = %d, catalog total = %d", sum, total)
	}

	// Subclass filter keeps the base class and drops the other subclasses.
	data, _ := c.ListClassFeatures("barbarian", "path of the berserker", 1, 100)
	sawBase, sawBerserker := false, false
	for _, f := range data {
		switch f.Subclass {
		case "":
			sawBase = true
		case "Path of the Berserker":
			sawBerserker = true
		default:
			t.Errorf("subclass filter leaked %q", f.Subclass)
		}
	}
	if !sawBase || !sawBerserker {
		t.Errorf("subclass filter: base=%v berserker=%v", sawBase, sawBerserker)
	}
	// Base class features come first, in level order.
	if len(data) == 0 || data[0].Subclass != "" || data[0].Level != 1 {
		t.Errorf("unexpected first feature: %+v", data)
	}

	rage := c.GetClassFeature("cf-barbarian-rage")
	if rage == nil || rage.Level != 1 || rage.Class != "Barbarian" {
		t.Errorf("cf-barbarian-rage wrong: %+v", rage)
	}
	if c.GetClassFeature("nope") != nil {
		t.Error("unknown class feature should be nil")
	}
}
