package activity

import (
	"net/http"
	"strings"

	"github.com/elad/rolebook-backend/internal/model"
)

// RouteInfo is a mutating route's default history description.
type RouteInfo struct {
	Type, Category, Title string
	// Skip marks a mutation that changes nothing user-visible.
	Skip bool
	// AccountLevel entries belong to no campaign, even when the URL names one.
	AccountLevel bool
}

// RouteKey builds a Registry key from a method and chi route pattern.
func RouteKey(method, pattern string) string {
	if len(pattern) > 1 {
		pattern = strings.TrimSuffix(pattern, "/")
	}
	return method + " " + pattern
}

// Registry describes every mutating route. A guard test fails when a route is missing.
var Registry = map[string]RouteInfo{
	"PATCH /api/players/{playerId}/coins":                                           {Type: "coins.adjust", Category: model.ActivityCategoryCoins, Title: "Coins adjusted"},
	"POST /api/players/{playerId}/coins/convert":                                    {Type: "coins.convert", Category: model.ActivityCategoryCoins, Title: "Coins converted"},
	"POST /api/players":                                                             {Type: "player.create", Category: model.ActivityCategoryCharacter, Title: "Character created"},
	"PATCH /api/players/{playerId}":                                                 {Type: "player.update", Category: model.ActivityCategoryCharacter, Title: "Character updated"},
	"DELETE /api/players/{playerId}":                                                {Type: "player.delete", Category: model.ActivityCategoryCharacter, Title: "Character deleted"},
	"POST /api/campaigns/{campaignId}/players":                                      {Type: "player.create_npc", Category: model.ActivityCategoryCharacter, Title: "NPC character created"},
	"POST /api/campaigns/{campaignId}/companions":                                   {Type: "companion.create", Category: model.ActivityCategoryCharacter, Title: "Companion added"},
	"PATCH /api/campaigns/{id}/players/{playerId}":                                  {Type: "campaign.player_active", Category: model.ActivityCategoryCharacter, Title: "Player status changed"},
	"POST /api/players/{playerId}/spells":                                           {Type: "spell.create", Category: model.ActivityCategorySpells, Title: "Added spell"},
	"PATCH /api/players/{playerId}/spells/{spellId}":                                {Type: "spell.update", Category: model.ActivityCategorySpells, Title: "Updated spell"},
	"DELETE /api/players/{playerId}/spells/{spellId}":                               {Type: "spell.delete", Category: model.ActivityCategorySpells, Title: "Removed spell"},
	"POST /api/players/{playerId}/spells/{spellId}/cast":                            {Type: "spell.cast", Category: model.ActivityCategorySpells, Title: "Cast spell"},
	"PUT /api/players/{playerId}/spell-slots":                                       {Type: "spellslots.update", Category: model.ActivityCategorySpells, Title: "Spell slots changed"},
	"POST /api/players/{playerId}/inventory":                                        {Type: "inventory.create", Category: model.ActivityCategoryItems, Title: "Added inventory item"},
	"PATCH /api/players/{playerId}/inventory/{equipmentId}":                         {Type: "inventory.update", Category: model.ActivityCategoryItems, Title: "Updated inventory item"},
	"DELETE /api/players/{playerId}/inventory/{equipmentId}":                        {Type: "inventory.delete", Category: model.ActivityCategoryItems, Title: "Removed inventory item"},
	"POST /api/players/{playerId}/limited-uses":                                     {Type: "limiteduse.create", Category: model.ActivityCategoryAbilities, Title: "Added limited use"},
	"PATCH /api/players/{playerId}/limited-uses/{limitedUseId}":                     {Type: "limiteduse.update", Category: model.ActivityCategoryAbilities, Title: "Updated limited use"},
	"DELETE /api/players/{playerId}/limited-uses/{limitedUseId}":                    {Type: "limiteduse.delete", Category: model.ActivityCategoryAbilities, Title: "Removed limited use"},
	"POST /api/players/{playerId}/limited-uses/{limitedUseId}/use":                  {Type: "limiteduse.use", Category: model.ActivityCategoryAbilities, Title: "Used ability"},
	"POST /api/players/{playerId}/limited-uses/{limitedUseId}/restore":              {Type: "limiteduse.restore", Category: model.ActivityCategoryAbilities, Title: "Restored ability use"},
	"POST /api/players/{playerId}/limited-uses/{limitedUseId}/reset":                {Type: "limiteduse.reset", Category: model.ActivityCategoryAbilities, Title: "Reset ability uses"},
	"POST /api/campaigns/{campaignId}/sessions":                                     {Type: "session.create", Category: model.ActivityCategorySchedule, Title: "Created session"},
	"PATCH /api/campaigns/{campaignId}/sessions/{sessionId}":                        {Type: "session.update", Category: model.ActivityCategorySchedule, Title: "Updated session"},
	"DELETE /api/campaigns/{campaignId}/sessions/{sessionId}":                       {Type: "session.delete", Category: model.ActivityCategorySchedule, Title: "Deleted session"},
	"PUT /api/campaigns/{campaignId}/sessions/{sessionId}/availability":             {Type: "availability.update", Category: model.ActivityCategorySchedule, Title: "Marked availability"},
	"DELETE /api/campaigns/{campaignId}/sessions/{sessionId}/availability":          {Type: "availability.delete", Category: model.ActivityCategorySchedule, Title: "Cleared availability"},
	"PUT /api/campaigns/{campaignId}/sessions/{sessionId}/confirmed-slot":           {Type: "session.confirm", Category: model.ActivityCategorySchedule, Title: "Confirmed session time"},
	"DELETE /api/campaigns/{campaignId}/sessions/{sessionId}/confirmed-slot":        {Type: "session.unconfirm", Category: model.ActivityCategorySchedule, Title: "Cleared session time"},
	"PUT /api/campaigns/{campaignId}/sessions/{sessionId}/my-notes":                 {Type: "session_note.update", Category: model.ActivityCategoryNotes, Title: "Edited session notes"},
	"PUT /api/campaigns/{campaignId}/content-notes/{targetType}/{entryId}":          {Type: "content_note.update", Category: model.ActivityCategoryNotes, Title: "Edited private note"},
	"POST /api/campaigns/{campaignId}/content-requests":                             {Type: "request.create", Category: model.ActivityCategoryRequests, Title: "Requested custom content"},
	"PATCH /api/campaigns/{campaignId}/content-requests/{id}":                       {Type: "request.update", Category: model.ActivityCategoryRequests, Title: "Edited content request"},
	"DELETE /api/campaigns/{campaignId}/content-requests/{id}":                      {Type: "request.withdraw", Category: model.ActivityCategoryRequests, Title: "Withdrew content request"},
	"POST /api/campaigns/{campaignId}/content-requests/{id}/approve":                {Type: "request.approve", Category: model.ActivityCategoryRequests, Title: "Content request approved"},
	"POST /api/campaigns/{campaignId}/content-requests/{id}/deny":                   {Type: "request.deny", Category: model.ActivityCategoryRequests, Title: "Content request denied"},
	"POST /api/campaigns/{campaignId}/custom-equipment":                             {Type: "custom_equipment.create", Category: model.ActivityCategoryItems, Title: "Created custom equipment"},
	"PATCH /api/campaigns/{campaignId}/custom-equipment/{id}":                       {Type: "custom_equipment.update", Category: model.ActivityCategoryItems, Title: "Updated custom equipment"},
	"DELETE /api/campaigns/{campaignId}/custom-equipment/{id}":                      {Type: "custom_equipment.delete", Category: model.ActivityCategoryItems, Title: "Deleted custom equipment"},
	"POST /api/campaigns/{campaignId}/custom-spells":                                {Type: "custom_spell.create", Category: model.ActivityCategorySpells, Title: "Created custom spell"},
	"PATCH /api/campaigns/{campaignId}/custom-spells/{id}":                          {Type: "custom_spell.update", Category: model.ActivityCategorySpells, Title: "Updated custom spell"},
	"DELETE /api/campaigns/{campaignId}/custom-spells/{id}":                         {Type: "custom_spell.delete", Category: model.ActivityCategorySpells, Title: "Deleted custom spell"},
	"POST /api/campaigns/{campaignId}/locations":                                    {Type: "location.create", Category: model.ActivityCategoryOther, Title: "Created location"},
	"PATCH /api/campaigns/{campaignId}/locations/{id}":                              {Type: "location.update", Category: model.ActivityCategoryOther, Title: "Updated location"},
	"DELETE /api/campaigns/{campaignId}/locations/{id}":                             {Type: "location.delete", Category: model.ActivityCategoryOther, Title: "Deleted location"},
	"POST /api/campaigns/{campaignId}/locations/{id}/share":                         {Type: "location.share", Category: model.ActivityCategoryOther, Title: "Shared location"},
	"POST /api/campaigns/{campaignId}/npcs":                                         {Type: "npc.create", Category: model.ActivityCategoryOther, Title: "Created NPC"},
	"PATCH /api/campaigns/{campaignId}/npcs/{id}":                                   {Type: "npc.update", Category: model.ActivityCategoryOther, Title: "Updated NPC"},
	"DELETE /api/campaigns/{campaignId}/npcs/{id}":                                  {Type: "npc.delete", Category: model.ActivityCategoryOther, Title: "Deleted NPC"},
	"POST /api/campaigns/{campaignId}/npcs/{id}/share":                              {Type: "npc.share", Category: model.ActivityCategoryOther, Title: "Shared NPC"},
	"POST /api/campaigns/{campaignId}/map-pins":                                     {Type: "map_pin.create", Category: model.ActivityCategoryOther, Title: "Added map pin"},
	"PATCH /api/campaigns/{campaignId}/map-pins/{id}":                               {Type: "map_pin.update", Category: model.ActivityCategoryOther, Title: "Updated map pin"},
	"DELETE /api/campaigns/{campaignId}/map-pins/{id}":                              {Type: "map_pin.delete", Category: model.ActivityCategoryOther, Title: "Removed map pin"},
	"POST /api/campaigns/{campaignId}/map-pins/{id}/share":                          {Type: "map_pin.share", Category: model.ActivityCategoryOther, Title: "Shared map pin"},
	"POST /api/campaigns/{campaignId}/initiative":                                   {Type: "initiative.start", Category: model.ActivityCategoryCombat, Title: "Started initiative"},
	"POST /api/campaigns/{campaignId}/initiative/submit":                            {Type: "initiative.submit", Category: model.ActivityCategoryCombat, Title: "Rolled initiative"},
	"POST /api/campaigns/{campaignId}/initiative/enemies":                           {Type: "initiative.enemy_add", Category: model.ActivityCategoryCombat, Title: "Added enemy to initiative"},
	"DELETE /api/campaigns/{campaignId}/initiative/enemies/{participantId}":         {Type: "initiative.enemy_remove", Category: model.ActivityCategoryCombat, Title: "Removed enemy from initiative"},
	"POST /api/campaigns/{campaignId}/initiative/participants/{participantId}/skip": {Type: "initiative.skip", Category: model.ActivityCategoryCombat, Title: "Skipped in initiative"},
	"POST /api/campaigns/{campaignId}/initiative/end-turn":                          {Type: "initiative.end_turn", Category: model.ActivityCategoryCombat, Title: "Ended turn"},
	"POST /api/campaigns/{campaignId}/initiative/resort":                            {Type: "initiative.resort", Category: model.ActivityCategoryCombat, Title: "Reordered initiative"},
	"POST /api/campaigns/{campaignId}/initiative/resolve":                           {Type: "initiative.resolve", Category: model.ActivityCategoryCombat, Title: "Ended combat"},
	"POST /api/campaigns":                                                           {Type: "campaign.create", Category: model.ActivityCategoryOther, Title: "Created campaign"},
	"PATCH /api/campaigns/{id}":                                                     {Type: "campaign.update", Category: model.ActivityCategoryOther, Title: "Updated campaign"},
	"DELETE /api/campaigns/{id}":                                                    {Type: "campaign.delete", Category: model.ActivityCategoryOther, Title: "Deleted campaign", AccountLevel: true},
	"POST /api/auth/change-password":                                                {Type: "account.password", Category: model.ActivityCategoryOther, Title: "Changed password", AccountLevel: true},
	"POST /api/auth/change-email":                                                   {Type: "account.email", Category: model.ActivityCategoryOther, Title: "Requested email change", AccountLevel: true},
	"POST /api/auth/verify-email-change":                                            {Type: "account.email_verify", Category: model.ActivityCategoryOther, Title: "Confirmed email change", AccountLevel: true},
	"POST /api/auth/resend-email-change":                                            {Type: "account.email_resend", Category: model.ActivityCategoryOther, Title: "Resent email change code", AccountLevel: true},
	"PUT /api/admin/arsenal/equipment/{equipmentId}/image":                          {Type: "admin.equipment_image_set", Category: model.ActivityCategoryOther, Title: "Set equipment image", AccountLevel: true},
	"DELETE /api/admin/arsenal/equipment/{equipmentId}/image":                       {Type: "admin.equipment_image_delete", Category: model.ActivityCategoryOther, Title: "Removed equipment image", AccountLevel: true},
	"PUT /api/admin/arsenal/spells/{spellId}/image":                                 {Type: "admin.spell_image_set", Category: model.ActivityCategoryOther, Title: "Set spell image", AccountLevel: true},
	"DELETE /api/admin/arsenal/spells/{spellId}/image":                              {Type: "admin.spell_image_delete", Category: model.ActivityCategoryOther, Title: "Removed spell image", AccountLevel: true},
	"POST /api/uploads/url":                                                         {Skip: true},
}

func routeInfo(method, pattern string) RouteInfo {
	if info, ok := Registry[RouteKey(method, pattern)]; ok {
		return info
	}
	title := "Updated something"
	switch method {
	case http.MethodPost:
		title = "Added something"
	case http.MethodDelete:
		title = "Removed something"
	}
	return RouteInfo{Type: "generic", Category: model.ActivityCategoryOther, Title: title}
}
