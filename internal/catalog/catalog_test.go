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
