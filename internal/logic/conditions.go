package logic

import (
	"strings"

	"github.com/elad/rolebook-backend/internal/model"
)

// conditionWarnings maps lowercase condition names to their mechanical warning text.
var conditionWarnings = map[string]string{
	"blinded":    "Disadvantage on attacks; attackers have advantage.",
	"invisible":  "Advantage on attacks; attackers have disadvantage.",
	"deafened":   "Automatically fail ability checks involving hearing.",
	"poisoned":   "Disadvantage on attack rolls and ability checks.",
	"frightened": "Disadvantage on checks and attacks while source is in sight; cannot move closer.",
	"prone":      "Disadvantage on attacks; attackers within 5ft have advantage, others have disadvantage.",
	"restrained": "Disadvantage on attacks and Dex saves; attackers have advantage.",
	"paralyzed":  "Automatically fail Str and Dex saves; attackers have advantage (crit if within 5ft).",
	"stunned":    "Automatically fail Str and Dex saves; attackers have advantage (crit if within 5ft).",
	"unconscious": "Automatically fail Str and Dex saves; attackers have advantage (crit if within 5ft).",
}

// exhaustionEffects maps exhaustion level to its mechanical effect description.
var exhaustionEffects = map[int]string{
	1: "Disadvantage on ability checks.",
	2: "Speed halved.",
	3: "Disadvantage on attack rolls and saving throws.",
	4: "Hit point maximum halved.",
	5: "Speed reduced to 0.",
	6: "Death.",
}

// speedZeroConditions is the set of lowercase condition names that reduce speed to 0.
var speedZeroConditions = map[string]bool{
	"grappled":   true,
	"restrained": true,
	"paralyzed":  true,
	"petrified":  true,
	"stunned":    true,
	"unconscious": true,
}

// CalculateDerivedStats computes effective stats and warnings based on a player's conditions.
func CalculateDerivedStats(p *model.Player) *model.DerivedStats {
	if p == nil {
		return nil
	}

	effectiveSpeed := p.Speed
	var warnings []model.ConditionWarning

	// 1. Speed-affecting conditions (case-insensitive)
	speedZero := p.ExhaustionLevel >= 5
	for cond, active := range p.Conditions {
		if active && speedZeroConditions[strings.ToLower(cond)] {
			speedZero = true
		}
	}
	if speedZero {
		effectiveSpeed = 0
	} else if p.ExhaustionLevel >= 2 {
		effectiveSpeed = p.Speed / 2
	}

	// 2. Condition warnings
	for cond, active := range p.Conditions {
		if !active {
			continue
		}
		if warning, ok := conditionWarnings[strings.ToLower(cond)]; ok {
			warnings = append(warnings, model.ConditionWarning{
				Condition: cond,
				Effect:    warning,
			})
		}
	}

	// 3. Exhaustion specific warnings
	if p.ExhaustionLevel > 0 {
		warnings = append(warnings, model.ConditionWarning{
			Condition: "Exhaustion",
			Effect:    exhaustionEffects[p.ExhaustionLevel],
		})
	}

	return &model.DerivedStats{
		EffectiveSpeed: effectiveSpeed,
		Warnings:       warnings,
	}
}
