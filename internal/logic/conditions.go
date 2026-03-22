package logic

import (
	"strings"

	"github.com/elad/rolebook-backend/internal/model"
)

// CalculateDerivedStats computes effective stats and warnings based on a player's conditions.
func CalculateDerivedStats(p *model.Player) *model.DerivedStats {
	if p == nil {
		return nil
	}

	effectiveSpeed := p.Speed
	var warnings []model.ConditionWarning

	// 1. Speed-affecting conditions
	if p.Conditions["Grappled"] || p.Conditions["Restrained"] || p.Conditions["Paralyzed"] ||
		p.Conditions["Petrified"] || p.Conditions["Stunned"] || p.Conditions["Unconscious"] ||
		p.ExhaustionLevel >= 5 {
		effectiveSpeed = 0
	} else if p.ExhaustionLevel >= 2 {
		effectiveSpeed = p.Speed / 2
	}

	// 2. Condition warnings
	for cond, active := range p.Conditions {
		if !active {
			continue
		}

		warning := getWarningForCondition(cond)
		if warning != "" {
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
			Effect:    getExhaustionEffect(p.ExhaustionLevel),
		})
	}

	return &model.DerivedStats{
		EffectiveSpeed: effectiveSpeed,
		Warnings:       warnings,
	}
}

func getWarningForCondition(cond string) string {
	switch strings.ToLower(cond) {
	case "blinded":
		return "Disadvantage on attacks; attackers have advantage."
	case "invisible":
		return "Advantage on attacks; attackers have disadvantage."
	case "deafened":
		return "Automatically fail ability checks involving hearing."
	case "poisoned":
		return "Disadvantage on attack rolls and ability checks."
	case "frightened":
		return "Disadvantage on checks and attacks while source is in sight; cannot move closer."
	case "prone":
		return "Disadvantage on attacks; attackers within 5ft have advantage, others have disadvantage."
	case "restrained":
		return "Disadvantage on attacks and Dex saves; attackers have advantage."
	case "paralyzed", "stunned", "unconscious":
		return "Automatically fail Str and Dex saves; attackers have advantage (crit if within 5ft)."
	default:
		return ""
	}
}

func getExhaustionEffect(level int) string {
	switch level {
	case 1:
		return "Disadvantage on ability checks."
	case 2:
		return "Speed halved."
	case 3:
		return "Disadvantage on attack rolls and saving throws."
	case 4:
		return "Hit point maximum halved."
	case 5:
		return "Speed reduced to 0."
	case 6:
		return "Death."
	default:
		return ""
	}
}
