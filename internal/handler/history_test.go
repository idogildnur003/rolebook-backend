package handler

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHistoryCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 11, 8, 20, 1, 2, 345000000, time.UTC)
	gotT, gotID, err := decodeHistoryCursor(encodeHistoryCursor(at, "abc|def"))
	if err != nil || !gotT.Equal(at) || gotID != "abc|def" {
		t.Fatalf("got %v %q %v", gotT, gotID, err)
	}
}

func TestParseHistoryQuery_LimitClamp(t *testing.T) {
	for raw, want := range map[string]int{"": 50, "10": 10, "100": 100, "101": 100, "0": 50, "-1": 50, "abc": 50} {
		f, err := parseHistoryQuery(url.Values{"limit": {raw}})
		if err != nil || f.Limit != want {
			t.Fatalf("limit %q: got %d err %v", raw, f.Limit, err)
		}
	}
}

func TestParseHistoryQuery_BadCursor(t *testing.T) {
	for _, raw := range []string{"%%%", "bm90LWEtY3Vyc29y"} {
		if _, err := parseHistoryQuery(url.Values{"before": {raw}}); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestParseHistoryQuery_Filters(t *testing.T) {
	f, err := parseHistoryQuery(url.Values{"session": {"between"}, "category": {"coins", "hp"}, "byDm": {"true"}, "q": {" gold "}})
	if err != nil || f.Session != "between" || len(f.Categories) != 2 || !f.ByDM || f.Query != "gold" {
		t.Fatalf("got %+v err %v", f, err)
	}
}

func TestParseHistoryQuery_IgnoresSubject(t *testing.T) {
	f, _ := parseHistoryQuery(url.Values{"subjectUserId": {"victim"}, "campaignId": {"other"}})
	if f.SubjectUserID != "" || f.CampaignID != "" {
		t.Fatalf("got %+v", f)
	}
}

func TestParseHistoryQuery_SearchIsSafeForMongo(t *testing.T) {
	f, err := parseHistoryQuery(url.Values{"q": {"go\x00ld"}})
	if err != nil || f.Query != "gold" {
		t.Fatalf("NUL: got %q err %v", f.Query, err)
	}
	f, err = parseHistoryQuery(url.Values{"q": {strings.Repeat("é", 40000)}})
	if err != nil || len([]rune(f.Query)) != historyMaxQueryRunes {
		t.Fatalf("long: got %d runes err %v", len([]rune(f.Query)), err)
	}
}
