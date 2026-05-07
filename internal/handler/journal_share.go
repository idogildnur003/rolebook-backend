package handler

import "github.com/elad/rolebook-backend/internal/model"

// visibilityAllowsRead encodes the read filter shared by all journal entities.
// The owner can always read; sharedWithAll opens to every member; sharedPlayerIds
// opens to specific members. Lives here (rather than in any single handler file)
// so all three entity handlers can share it without an import cycle.
func visibilityAllowsRead(ownerPlayerID string, v model.Visibility, callerPlayerID string) bool {
	if ownerPlayerID == callerPlayerID {
		return true
	}
	if v.SharedWithAll {
		return true
	}
	for _, id := range v.SharedPlayerIds {
		if id == callerPlayerID {
			return true
		}
	}
	return false
}

// isValidVisibilityPatch checks that v decodes to the expected
// {sharedWithAll bool, sharedPlayerIds []string} shape. Used by all three
// journal Update handlers to reject malformed visibility patches before they
// reach Mongo.
//
// Tolerates a partial patch (only one of the two fields present). The two
// fields are independently optional in JSON; both being absent is a no-op
// patch. Extra keys are silently ignored — the caller's $set will simply not
// touch them.
func isValidVisibilityPatch(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if sw, ok := m["sharedWithAll"]; ok {
		if _, isBool := sw.(bool); !isBool {
			return false
		}
	}
	if ids, ok := m["sharedPlayerIds"]; ok {
		arr, isArr := ids.([]any)
		if !isArr {
			return false
		}
		for _, e := range arr {
			if _, isString := e.(string); !isString {
				return false
			}
		}
	}
	return true
}
