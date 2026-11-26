package handler

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/model"
)

// playerUpdateDesc describes a sheet edit. A change touching only hit points
// gets the HP category, and one touching only spell slots is logged like the
// spell-slots route, so each appears under its filter and session summary.
func playerUpdateDesc(before, after *model.Player) activity.Desc {
	changes := activity.DiffPlayerPatch(before, after)
	d := activity.Desc{Category: model.ActivityCategoryCharacter, Changes: changes}
	if len(changes) == 0 {
		return d
	}
	if slots := activity.DiffSpellSlots(before.SpellSlots, after.SpellSlots); len(slots) == len(changes) {
		sd := spellSlotDesc(before.SpellSlots, after.SpellSlots)
		sd.Type, sd.Category = "spellslots.update", model.ActivityCategorySpells
		return sd
	}
	for _, c := range changes {
		if c.Field != "HP" && c.Field != "max HP" && c.Field != "temp HP" {
			return d
		}
	}
	d.Category, d.Title = model.ActivityCategoryHP, "HP changed"
	return d
}

// describeOrSkip records d, or no entry at all when the request changed nothing
// a person could see.
func describeOrSkip(r *http.Request, d activity.Desc) {
	if len(d.Changes) == 0 {
		activity.Skip(r)
		return
	}
	activity.Describe(r, d)
}

// updateDesc names the edited entry and lists its changed fields.
func updateDesc(name string, before, after any, hide ...string) activity.Desc {
	return activity.Desc{Detail: name, Changes: activity.DiffFields(before, after, hide...)}
}

// campaignUpdateDesc leaves out the member and session lists, which change
// through their own routes, and the map size, which follows the map image.
func campaignUpdateDesc(before, after *model.Campaign) activity.Desc {
	return updateDesc(after.Name, before, after, "sessions", "members", "mapImageWidth", "mapImageHeight")
}

// sessionUpdateDesc compares the session as it was in the campaign with the
// updated one. Scheduling changes through its own routes.
func sessionUpdateDesc(c *model.Campaign, after *model.Session) activity.Desc {
	for i := range c.Sessions {
		if c.Sessions[i].ID == after.ID {
			return updateDesc(after.Name, &c.Sessions[i], after, "schedule")
		}
	}
	return activity.Desc{Detail: after.Name}
}

// requestEditDesc lists the edited proposal fields. Review bookkeeping is
// left out; it changes only when the DM decides.
func requestEditDesc(before, after *model.ContentRequest) activity.Desc {
	d := requestDecisionDesc(after)
	d.Changes = activity.DiffFields(before, after, "status", "resolvedAt", "resolvedByUserId", "resultId",
		"proposedByUserId", "proposedByName", "kind", "targetType")
	return d
}

// limitedUseUpdateDesc lists edited fields. The private note's text is never
// logged, only that it changed.
func limitedUseUpdateDesc(list model.LimitedUseList, id string, after *model.LimitedUse) activity.Desc {
	d := limitedUseDesc(list, id, after)
	var before *model.LimitedUse
	for i := range list {
		if list[i].ID == id {
			before = &list[i]
		}
	}
	if before == nil || after == nil {
		return d
	}
	d.Changes = activity.DiffFields(before, after, "note")
	if before.Note != after.Note {
		d.Changes = append(d.Changes, model.ActivityChange{Field: "note"})
	}
	return d
}

// playerDeleteDesc captures identity up front: the character cannot be looked
// up once the handler has deleted it. A companion is linked to its owner's user.
func playerDeleteDesc(p *model.Player) activity.Desc {
	return activity.Desc{
		CampaignID: p.CampaignID, PlayerID: p.ID, PlayerName: p.Name,
		SubjectUserID: p.LinkedUserID, Detail: p.Name,
	}
}

func playerCreatedDesc(p *model.Player) activity.Desc {
	return activity.Desc{
		CampaignID: p.CampaignID, PlayerID: p.ID, PlayerName: p.Name,
		SubjectUserID: p.LinkedUserID, Detail: p.Name,
	}
}

// spellSlotDesc logs slots remaining per level, and max where it changed.
func spellSlotDesc(before, after map[string]model.SpellSlot) activity.Desc {
	d := activity.Desc{Changes: activity.DiffSpellSlots(before, after)}
	spent, restored := false, false
	for _, c := range d.Changes {
		if strings.HasSuffix(c.Field, " max slots") {
			continue
		}
		b, _ := c.Before.(int)
		a, _ := c.After.(int)
		spent = spent || a < b
		restored = restored || a > b
	}
	switch {
	case spent && !restored:
		d.Title = "Used spell slot"
	case restored && !spent:
		d.Title = "Restored spell slots"
	}
	return d
}

// spellCastDesc names the spell. When a slot was spent it records that level's
// remaining slots, deriving the before value from the atomically returned
// after so a concurrent edit cannot make it wrong.
func spellCastDesc(name string, slotLevel int, after map[string]model.SpellSlot) activity.Desc {
	d := activity.Desc{Detail: name}
	if slotLevel == 0 {
		return d
	}
	level := strconv.Itoa(slotLevel)
	now := after[level].Max - after[level].Used
	d.Changes = []model.ActivityChange{{Field: "L" + level + " slots", Before: now + 1, After: now}}
	return d
}

// shiftCoin returns c with delta added to one denomination.
func shiftCoin(c model.Coins, key string, delta int) model.Coins {
	switch key {
	case "cp":
		c.CP += delta
	case "sp":
		c.SP += delta
	case "ep":
		c.EP += delta
	case "gp":
		c.GP += delta
	case "pp":
		c.PP += delta
	}
	return c
}

// coinsAdjustDesc derives the old purse from the atomically returned new one,
// so concurrent edits cannot make the logged before value wrong.
func coinsAdjustDesc(after model.Coins, deltas map[string]int) activity.Desc {
	before := after
	for key, delta := range deltas {
		before = shiftCoin(before, key, -delta)
	}
	return activity.Desc{Changes: activity.DiffCoins(before, after)}
}

// coinsConvertDesc lists the spent denomination first.
func coinsConvertDesc(after model.Coins, from, to string, amount, gained int) activity.Desc {
	before := shiftCoin(shiftCoin(after, from, amount), to, -gained)
	changes := activity.DiffCoins(before, after)
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].Field == from && changes[j].Field != from
	})
	return activity.Desc{Changes: changes}
}

// limitedUseDesc compares remaining uses. A nil after means the entry is gone.
// The private note is never logged.
func limitedUseDesc(list model.LimitedUseList, id string, after *model.LimitedUse) activity.Desc {
	var before *model.LimitedUse
	for i := range list {
		if list[i].ID == id {
			before = &list[i]
			break
		}
	}
	var d activity.Desc
	switch {
	case after != nil:
		d.Detail = after.Name
	case before != nil:
		d.Detail = before.Name
	}
	if before == nil {
		return d
	}
	if after == nil {
		d.Changes = []model.ActivityChange{{Field: "uses", Before: before.Remaining, After: nil}}
	} else if after.Remaining != before.Remaining {
		d.Changes = []model.ActivityChange{{Field: "uses", Before: before.Remaining, After: after.Remaining}}
	}
	return d
}

// spellDesc names the spell and records a prepared toggle from a PATCH body.
func spellDesc(spells []model.PlayerSpell, id string, patch map[string]any) activity.Desc {
	for i := range spells {
		if spells[i].SpellID != id {
			continue
		}
		d := activity.Desc{Detail: spells[i].Name}
		if prepared, ok := patch["isPrepared"].(bool); ok && prepared != spells[i].IsPrepared {
			d.Title = "Unprepared spell"
			if prepared {
				d.Title = "Prepared spell"
			}
			d.Changes = []model.ActivityChange{{Field: "prepared", Before: spells[i].IsPrepared, After: prepared}}
		}
		return d
	}
	return activity.Desc{}
}

func inventoryCreateDesc(item model.PlayerInventoryItem) activity.Desc {
	return activity.Desc{
		Detail:  item.Name,
		Changes: []model.ActivityChange{{Field: "qty", Before: nil, After: item.Quantity}},
	}
}

func findInventoryItem(items []model.PlayerInventoryItem, id string) *model.PlayerInventoryItem {
	for i := range items {
		if items[i].EquipmentID == id {
			return &items[i]
		}
	}
	return nil
}

func inventoryUpdateDesc(items []model.PlayerInventoryItem, id string, patch map[string]any) activity.Desc {
	item := findInventoryItem(items, id)
	if item == nil {
		return activity.Desc{}
	}
	d := activity.Desc{Detail: item.Name}
	if qty, ok := toInt(patch["quantity"]); ok && qty != item.Quantity {
		d.Changes = []model.ActivityChange{{Field: "qty", Before: item.Quantity, After: qty}}
	}
	if name, ok := patch["name"].(string); ok && name != item.Name {
		d.Changes = append(d.Changes, model.ActivityChange{Field: "name", Before: item.Name, After: name})
	}
	return d
}

func inventoryDeleteDesc(items []model.PlayerInventoryItem, id string) activity.Desc {
	item := findInventoryItem(items, id)
	if item == nil {
		return activity.Desc{}
	}
	return activity.Desc{
		Detail:  item.Name,
		Changes: []model.ActivityChange{{Field: "qty", Before: item.Quantity, After: nil}},
	}
}

// sessionDesc names a session as it is at request time.
func sessionDesc(c *model.Campaign, sessionID string) activity.Desc {
	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			return activity.Desc{Detail: c.Sessions[i].Name}
		}
	}
	return activity.Desc{}
}

// sessionNoteDesc records which session's note changed, never its text.
func sessionNoteDesc(c *model.Campaign, sessionID string) activity.Desc {
	return sessionDesc(c, sessionID)
}

// contentNoteDesc records only the kind of entry the private note is on.
func contentNoteDesc(targetType string) activity.Desc {
	if targetType == "spell" {
		return activity.Desc{Detail: "On a spell"}
	}
	return activity.Desc{Detail: "On an item"}
}

func contentRequestName(req *model.ContentRequest) string {
	switch {
	case req.ItemPayload != nil:
		return req.ItemPayload.Name
	case req.SpellPayload != nil:
		return req.SpellPayload.Name
	}
	return ""
}

// requestDecisionDesc puts a DM's action on a request in the proposer's history.
func requestDecisionDesc(req *model.ContentRequest) activity.Desc {
	return activity.Desc{SubjectUserID: req.ProposedByUserID, Detail: contentRequestName(req)}
}

func requestWithdrawDesc(req *model.ContentRequest) activity.Desc {
	return activity.Desc{Detail: contentRequestName(req)}
}

// worldDesc names a location, NPC or map pin. These are owned by the member
// acting on them, so the entry stays in that member's own history.
func worldDesc(name string) activity.Desc {
	return activity.Desc{Detail: name}
}

// initiativeSubmitDesc targets the rolled-for character, so a DM rolling for a
// player appears in that player's history.
func initiativeSubmitDesc(playerID string, before *int, after int) activity.Desc {
	change := model.ActivityChange{Field: "initiative", After: after}
	if before != nil {
		change.Before = *before
	}
	return activity.Desc{PlayerID: playerID, Changes: []model.ActivityChange{change}}
}

func initiativeSkipDesc(p *model.InitiativeParticipant, skipped bool) activity.Desc {
	d := activity.Desc{Title: "Resumed in initiative", Detail: p.Name}
	if skipped {
		d.Title = "Skipped in initiative"
	}
	if p.ParticipantType == "player" {
		d.PlayerID = p.PlayerID
	}
	return d
}

// maxListedNames caps how many combatants a history line names.
const maxListedNames = 6

// participantNames lists combatants by name in the given order: by ids when
// ids is non-nil, else every participant.
func participantNames(c *model.InitiativeCall, ids []string) string {
	byID := make(map[string]string, len(c.Participants))
	for _, p := range c.Participants {
		byID[p.ID] = p.Name
	}
	if ids == nil {
		for _, p := range c.Participants {
			ids = append(ids, p.ID)
		}
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if n := byID[id]; n != "" {
			names = append(names, n)
		}
	}
	if len(names) > maxListedNames {
		return strings.Join(names[:maxListedNames], ", ") + fmt.Sprintf(" and %d more", len(names)-maxListedNames)
	}
	return strings.Join(names, ", ")
}

// initiativeCallDesc names everyone in the fight.
func initiativeCallDesc(c *model.InitiativeCall) activity.Desc {
	return activity.Desc{Detail: participantNames(c, nil)}
}

// enemyInitiativeDesc records the enemy's roll. A re-roll for an enemy already
// in the fight keeps its previous value as before.
func enemyInitiativeDesc(name string, before *int, after int) activity.Desc {
	change := model.ActivityChange{Field: "initiative", After: after}
	d := activity.Desc{Detail: name, Changes: []model.ActivityChange{change}}
	if before != nil {
		d.Changes[0].Before = *before
		d.Title = "Re-rolled enemy initiative"
	}
	return d
}

// endTurnDesc files the entry under the character whose turn ended, so a DM
// ending a player's turn shows in that player's history.
func endTurnDesc(c *model.InitiativeCall, endedID, nextID string) activity.Desc {
	var ended, next model.InitiativeParticipant
	for _, p := range c.Participants {
		switch p.ID {
		case endedID:
			ended = p
		case nextID:
			next = p
		}
	}
	d := activity.Desc{Detail: ended.Name}
	if endedID != nextID {
		d.Changes = []model.ActivityChange{{Field: "turn", Before: ended.Name, After: next.Name}}
	}
	if ended.ParticipantType == "player" {
		d.PlayerID = ended.PlayerID
	}
	return d
}

// resortDesc shows the turn order before and after. Same order: no changes.
func resortDesc(c *model.InitiativeCall, before, after []string) activity.Desc {
	d := activity.Desc{Detail: participantNames(c, after)}
	if !slices.Equal(before, after) {
		d.Changes = []model.ActivityChange{{Field: "turn order", Before: participantNames(c, before), After: participantNames(c, after)}}
	}
	return d
}
