package handler

import (
	"fmt"
	"sort"

	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/model"
)

// playerUpdateDesc describes a sheet edit. A change touching only hit points
// gets the HP category so it appears under that filter.
func playerUpdateDesc(before, after *model.Player) activity.Desc {
	changes := activity.DiffPlayerPatch(before, after)
	d := activity.Desc{Category: model.ActivityCategoryCharacter, Changes: changes}
	if len(changes) == 0 {
		return d
	}
	for _, c := range changes {
		if c.Field != "HP" && c.Field != "max HP" && c.Field != "temp HP" {
			return d
		}
	}
	d.Category, d.Title = model.ActivityCategoryHP, "HP changed"
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

// spellSlotDesc logs slots remaining per level.
func spellSlotDesc(before, after map[string]model.SpellSlot) activity.Desc {
	levels := make([]string, 0, len(after))
	seen := map[string]bool{}
	for _, m := range []map[string]model.SpellSlot{before, after} {
		for level := range m {
			if !seen[level] {
				seen[level] = true
				levels = append(levels, level)
			}
		}
	}
	sort.Strings(levels)
	var d activity.Desc
	spent, restored := false, false
	for _, level := range levels {
		was, now := before[level].Max-before[level].Used, after[level].Max-after[level].Used
		if was == now {
			continue
		}
		spent = spent || now < was
		restored = restored || now > was
		d.Changes = append(d.Changes, model.ActivityChange{Field: fmt.Sprintf("L%s slots", level), Before: was, After: now})
	}
	switch {
	case spent && !restored:
		d.Title = "Used spell slot"
	case restored && !spent:
		d.Title = "Restored spell slots"
	}
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
