package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestChatVisible(t *testing.T) {
	c := client.NewFake() // seeds label "1" onto 1234567890@s.whatsapp.net

	if !chatVisible(c, client.Chat{JID: "1112223333@s.whatsapp.net"}, chatFilter{}) {
		t.Fatal(`filterAll should match every chat`)
	}
	if !chatVisible(c, client.Chat{JID: "1234567890@s.whatsapp.net"}, chatFilter{Kind: filterLabel, LabelID: "1"}) {
		t.Fatal("chat carrying label 1 should match filterLabel{1}")
	}
	if chatVisible(c, client.Chat{JID: "1112223333@s.whatsapp.net"}, chatFilter{Kind: filterLabel, LabelID: "1"}) {
		t.Fatal("chat without label 1 should not match filterLabel{1}")
	}
}

func TestChatMatchesFilter(t *testing.T) {
	unread := client.Chat{UnreadCount: 3}
	read := client.Chat{UnreadCount: 0}
	pinned := client.Chat{Pinned: true}
	group := client.Chat{IsGroup: true}

	cases := []struct {
		name     string
		chat     client.Chat
		onLabels []string
		filter   chatFilter
		want     bool
	}{
		{"all matches unread", unread, nil, chatFilter{Kind: filterAll}, true},
		{"all matches read", read, nil, chatFilter{Kind: filterAll}, true},
		{"unread matches unread chat", unread, nil, chatFilter{Kind: filterUnread}, true},
		{"unread rejects read chat", read, nil, chatFilter{Kind: filterUnread}, false},
		{"favorites matches pinned", pinned, nil, chatFilter{Kind: filterFavorites}, true},
		{"favorites rejects unpinned", read, nil, chatFilter{Kind: filterFavorites}, false},
		{"groups matches group chat", group, nil, chatFilter{Kind: filterGroups}, true},
		{"groups rejects 1:1 chat", read, nil, chatFilter{Kind: filterGroups}, false},
		{"label matches carried label", client.Chat{}, []string{"1", "2"}, chatFilter{Kind: filterLabel, LabelID: "1"}, true},
		{"label rejects uncarried label", client.Chat{}, []string{"2"}, chatFilter{Kind: filterLabel, LabelID: "1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chatMatchesFilter(tc.chat, tc.onLabels, tc.filter); got != tc.want {
				t.Errorf("chatMatchesFilter(%+v, %v, %+v) = %v, want %v", tc.chat, tc.onLabels, tc.filter, got, tc.want)
			}
		})
	}
}

func TestComputeChatCounts(t *testing.T) {
	chats := []client.Chat{
		{UnreadCount: 2},
		{UnreadCount: 0},
		{IsGroup: true},
		{IsGroup: true, UnreadCount: 1},
	}
	counts := computeChatCounts(chats)
	if counts.Unread != 2 {
		t.Errorf("Unread = %d, want 2", counts.Unread)
	}
	if counts.Groups != 2 {
		t.Errorf("Groups = %d, want 2", counts.Groups)
	}
}

func TestComputeLabelCounts(t *testing.T) {
	counts := computeLabelCounts(map[string][]string{
		"chatA": {"1", "2"},
		"chatB": {"1"},
		"chatC": {},
	})
	if counts["1"] != 2 {
		t.Errorf(`counts["1"] = %d, want 2`, counts["1"])
	}
	if counts["2"] != 1 {
		t.Errorf(`counts["2"] = %d, want 1`, counts["2"])
	}
	if counts["3"] != 0 {
		t.Errorf(`counts["3"] = %d, want 0`, counts["3"])
	}
}

func TestBuildChips(t *testing.T) {
	labels := []client.Label{{ID: "1", Name: "Work"}}

	t.Run("fixed chips only when no label active", func(t *testing.T) {
		chips := buildChips(chatCounts{Unread: 19, Groups: 7}, chatFilter{Kind: filterAll}, nil, labels)
		if len(chips) != 4 {
			t.Fatalf("len(chips) = %d, want 4", len(chips))
		}
		if !chips[0].Active {
			t.Error(`"All" chip should be active`)
		}
		if chips[1].Text != "Unread" || chips[1].Count != 19 {
			t.Errorf(`chips[1] = %q/%d, want "Unread"/19`, chips[1].Text, chips[1].Count)
		}
		if chips[3].Text != "Groups" || chips[3].Count != 7 {
			t.Errorf(`chips[3] = %q/%d, want "Groups"/7`, chips[3].Text, chips[3].Count)
		}
	})

	t.Run("zero counts omit the number", func(t *testing.T) {
		chips := buildChips(chatCounts{}, chatFilter{Kind: filterAll}, nil, labels)
		if chips[1].Count != 0 {
			t.Errorf(`chips[1].Count = %d, want 0`, chips[1].Count)
		}
		if chips[3].Count != 0 {
			t.Errorf(`chips[3].Count = %d, want 0`, chips[3].Count)
		}
	})

	t.Run("active label appears inline and active", func(t *testing.T) {
		filter := chatFilter{Kind: filterLabel, LabelID: "1"}
		chips := buildChips(chatCounts{}, filter, map[string]int{"1": 3}, labels)
		if len(chips) != 5 {
			t.Fatalf("len(chips) = %d, want 5 (4 fixed + inline label)", len(chips))
		}
		last := chips[4]
		if !last.Active {
			t.Error("inline label chip should be active")
		}
		if last.Text != "Work" || last.Count != 3 {
			t.Errorf("inline label chip = %q/%d, want \"Work\"/3", last.Text, last.Count)
		}
		for _, c := range chips[:4] {
			if c.Active {
				t.Errorf("fixed chip %q should not be active while a label filter is set", c.Key)
			}
		}
	})
}

func TestFilterForChipKey(t *testing.T) {
	cases := map[string]chatFilter{
		"all":       {Kind: filterAll},
		"unread":    {Kind: filterUnread},
		"favorites": {Kind: filterFavorites},
		"groups":    {Kind: filterGroups},
		"label:42":  {Kind: filterLabel, LabelID: "42"},
	}
	for key, want := range cases {
		if got := filterForChipKey(key); got != want {
			t.Errorf("filterForChipKey(%q) = %+v, want %+v", key, got, want)
		}
	}
}

func TestLabelColorHex(t *testing.T) {
	if labelColorHex(0) == "" {
		t.Error("labelColorHex(0) should not be empty")
	}
	// Out-of-range and negative indices should still resolve (wrap), not panic.
	if labelColorHex(1000) == "" {
		t.Error("labelColorHex(1000) should not be empty")
	}
	if labelColorHex(-1) == "" {
		t.Error("labelColorHex(-1) should not be empty")
	}
}
