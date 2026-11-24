package activity

import (
	"slices"
	"sort"

	"github.com/elad/rolebook-backend/internal/model"
)

func appendIntChange(out []model.ActivityChange, field string, before, after int) []model.ActivityChange {
	if before == after {
		return out
	}
	return append(out, model.ActivityChange{Field: field, Before: before, After: after})
}

// DiffCoins returns one change per denomination whose count changed.
func DiffCoins(before, after model.Coins) []model.ActivityChange {
	var out []model.ActivityChange
	out = appendIntChange(out, "cp", before.CP, after.CP)
	out = appendIntChange(out, "sp", before.SP, after.SP)
	out = appendIntChange(out, "ep", before.EP, after.EP)
	out = appendIntChange(out, "gp", before.GP, after.GP)
	out = appendIntChange(out, "pp", before.PP, after.PP)
	return out
}

// DiffPlayerPatch compares two versions of a character sheet. Free-text fields
// record only that they changed, so private prose never enters the history.
func DiffPlayerPatch(before, after *model.Player) []model.ActivityChange {
	var out []model.ActivityChange
	if before.Name != after.Name {
		out = append(out, model.ActivityChange{Field: "name", Before: before.Name, After: after.Name})
	}
	for _, f := range []struct {
		field string
		b, a  int
	}{
		{"level", before.Level, after.Level},
		{"XP", before.ExperiencePoints, after.ExperiencePoints},
		{"HP", before.CurrentHP, after.CurrentHP},
		{"max HP", before.MaxHP, after.MaxHP},
		{"temp HP", before.TempHP, after.TempHP},
		{"AC", before.AC, after.AC},
		{"speed", before.Speed, after.Speed},
		{"death save successes", before.DeathSaveSuccesses, after.DeathSaveSuccesses},
		{"death save failures", before.DeathSaveFailures, after.DeathSaveFailures},
		{"hit dice spent", before.HitDiceSpent, after.HitDiceSpent},
		{"hit dice max", before.HitDiceMax, after.HitDiceMax},
		{"proficiency bonus", before.ProficiencyBonus, after.ProficiencyBonus},
		{"expertise bonus", before.ExpertiseBonus, after.ExpertiseBonus},
		{"initiative bonus", before.InitiativeBonus, after.InitiativeBonus},
		{"spell save DC", before.SpellcastingSaveDC, after.SpellcastingSaveDC},
		{"spell modifier", before.SpellcastingModifier, after.SpellcastingModifier},
		{"spell attack", before.SpellcastingAttackBonus, after.SpellcastingAttackBonus},
	} {
		out = appendIntChange(out, f.field, f.b, f.a)
	}
	if before.HeroicInspiration != after.HeroicInspiration {
		out = append(out, model.ActivityChange{Field: "inspiration", Before: before.HeroicInspiration, After: after.HeroicInspiration})
	}
	for _, k := range unionKeys(before.AbilityScores, after.AbilityScores) {
		out = appendIntChange(out, k, before.AbilityScores[k], after.AbilityScores[k])
	}
	for _, k := range unionKeys(before.Conditions, after.Conditions) {
		if before.Conditions[k] != after.Conditions[k] {
			out = append(out, model.ActivityChange{Field: k, Before: before.Conditions[k], After: after.Conditions[k]})
		}
	}
	for _, f := range []struct {
		field string
		b, a  *string
	}{
		{"class", before.ClassName, after.ClassName},
		{"subclass", before.Subclass, after.Subclass},
		{"alignment", before.Alignment, after.Alignment},
		{"species", before.SpeciesOrRegion, after.SpeciesOrRegion},
		{"region", before.Region, after.Region},
		{"hit die", before.HitDie, after.HitDie},
		{"spellcasting ability", before.SpellcastingAbility, after.SpellcastingAbility},
	} {
		if derefOrEmpty(f.b) != derefOrEmpty(f.a) {
			out = append(out, model.ActivityChange{Field: f.field, Before: optionalValue(f.b), After: optionalValue(f.a)})
		}
	}
	if before.Race != after.Race {
		out = append(out, model.ActivityChange{Field: "race", Before: before.Race, After: after.Race})
	}
	if before.Size != after.Size {
		out = append(out, model.ActivityChange{Field: "size", Before: before.Size, After: after.Size})
	}
	for _, f := range []struct {
		field string
		b, a  []string
	}{
		{"saving throw proficiencies", before.ProficientSavingThrows, after.ProficientSavingThrows},
		{"skill proficiencies", before.ProficientSkills, after.ProficientSkills},
		{"skill expertise", before.ExpertiseSkills, after.ExpertiseSkills},
		{"features and feats", before.FeaturesAndFeats, after.FeaturesAndFeats},
		{"armor training", before.ArmorTraining, after.ArmorTraining},
	} {
		if !slices.Equal(f.b, f.a) {
			out = append(out, model.ActivityChange{Field: f.field})
		}
	}
	for _, f := range []struct {
		field string
		b, a  string
	}{
		{"notes", before.Notes, after.Notes},
		{"background", before.BackgroundStory, after.BackgroundStory},
		{"appearance", before.Appearance, after.Appearance},
		{"weapon proficiencies", before.WeaponProficiencies, after.WeaponProficiencies},
		{"tool proficiencies", before.ToolProficiencies, after.ToolProficiencies},
		{"languages", before.Languages, after.Languages},
		{"senses", before.Senses, after.Senses},
		{"portrait", before.AvatarURI, after.AvatarURI},
	} {
		if f.b != f.a {
			out = append(out, model.ActivityChange{Field: f.field})
		}
	}
	return out
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// optionalValue stores an unset or empty field as nil, shown as "—".
func optionalValue(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func unionKeys[V any](a, b map[string]V) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
