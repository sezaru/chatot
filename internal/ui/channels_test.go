package ui

import (
	"strings"
	"testing"
	"time"

	"chatot/internal/client"
)

func TestNewsletterRowVM(t *testing.T) {
	vm := newsletterRowVM(client.Newsletter{Name: "Chatot News", Description: "  Release notes  ", Muted: true})
	if vm.Name != "Chatot News" {
		t.Errorf("Name = %q", vm.Name)
	}
	if vm.Snippet != "Release notes" {
		t.Errorf("Snippet = %q, want trimmed", vm.Snippet)
	}
	if !vm.Muted {
		t.Errorf("Muted = false, want true")
	}
	if vm.Initial != "C" {
		t.Errorf("Initial = %q, want C", vm.Initial)
	}

	empty := newsletterRowVM(client.Newsletter{})
	if empty.Name != "Unknown channel" {
		t.Errorf("empty name fallback = %q", empty.Name)
	}
	if empty.Initial != "U" {
		t.Errorf("empty initial = %q", empty.Initial)
	}
}

func TestNewsletterPostVM(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-2 * time.Hour).Unix()
	m := client.NewsletterMessage{
		Text:      "Hello world",
		TS:        ts,
		Views:     42,
		Reactions: map[string]int{"👍": 8, "❤️": 3, "🎉": 3, "🔥": 1},
	}
	vm := newsletterPostVM(m, now)
	if vm.Text != "Hello world" {
		t.Errorf("Text = %q", vm.Text)
	}
	if vm.Views != "42 views" {
		t.Errorf("Views = %q", vm.Views)
	}
	if vm.TimeText != "10:00" {
		t.Errorf("TimeText = %q, want 10:00", vm.TimeText)
	}
	// Top three by count desc, emoji asc for ties (❤️ vs 🎉 both 3).
	if vm.Reactions != "👍 8   ❤️ 3   🎉 3" {
		t.Errorf("Reactions = %q", vm.Reactions)
	}
}

func TestNewsletterPostVMEmpty(t *testing.T) {
	now := time.Now()
	vm := newsletterPostVM(client.NewsletterMessage{Text: "   ", Views: 0}, now)
	if vm.Text != "(no text)" {
		t.Errorf("empty text = %q, want placeholder", vm.Text)
	}
	if vm.Reactions != "" {
		t.Errorf("empty reactions = %q", vm.Reactions)
	}
}

func TestMuteActionLabel(t *testing.T) {
	if muteActionLabel(true) != "Unmute" {
		t.Errorf("muted label wrong")
	}
	if muteActionLabel(false) != "Mute" {
		t.Errorf("unmuted label wrong")
	}
}

func TestDiscoverHelpers(t *testing.T) {
	list := []client.Newsletter{
		{ID: "a", Name: "Small", Subscribers: 10, Category: "Food"},
		{ID: "b", Name: "Big", Subscribers: 1000, Category: "News"},
		{ID: "c", Name: "Mid", Subscribers: 500, Category: "Food"},
	}
	all := filterDiscover(list, "All")
	if len(all) != 3 || all[0].ID != "b" || all[2].ID != "a" {
		t.Errorf("filterDiscover(All) = %+v", all)
	}
	food := filterDiscover(list, "Food")
	if len(food) != 2 || food[0].ID != "c" {
		t.Errorf("filterDiscover(Food) = %+v", food)
	}
	if got := discoverHeaderText("", "All", 3); got != "Most followed" {
		t.Errorf("header(All) = %q", got)
	}
	if got := discoverHeaderText("", "News", 1); got != "News" {
		t.Errorf("header(News) = %q", got)
	}
	if got := discoverHeaderText("zz", "All", 0); got != "0 results" {
		t.Errorf("header(query) = %q", got)
	}
	if got := discoverEmptyText("zzzz", "All"); got != "No channels match “zzzz”" {
		t.Errorf("empty(query) = %q", got)
	}
	if got := discoverEmptyText("", "Sports"); got != "Nothing in Sports yet" {
		t.Errorf("empty(cat) = %q", got)
	}
}

func TestChannelLabels(t *testing.T) {
	if followLabel(true) != "Following" || followLabel(false) != "Follow" {
		t.Errorf("followLabel")
	}
	if channelsCaptionText(3) != "Following · 3" {
		t.Errorf("channelsCaptionText")
	}
	n := client.Newsletter{Subscribers: 4218, Category: "Technology"}
	if got := channelInfoMeta(n); got != "4,218 followers · Technology" {
		t.Errorf("channelInfoMeta = %q", got)
	}
	if got := newsletterPostVM(client.NewsletterMessage{Text: "x", Views: 1204, TS: 0}, time.Now()).Meta; !strings.HasSuffix(got, " · 1,204 views") {
		t.Errorf("post meta = %q", got)
	}
	vm := newsletterRowVM(client.Newsletter{Name: "chatot releases", Verified: true})
	if !vm.Verified || vm.Initial != "C" {
		t.Errorf("row vm = %+v", vm)
	}
}

func TestApplyReactionChange(t *testing.T) {
	counts := map[string]int{"🔥": 1, "❤️": 48}
	applyReactionChange(counts, "", "🔥") // add ours
	if counts["🔥"] != 2 {
		t.Errorf("add: 🔥 = %d, want 2", counts["🔥"])
	}
	applyReactionChange(counts, "🔥", "🔥") // the same again: no drift
	if counts["🔥"] != 2 {
		t.Errorf("same again: 🔥 = %d, want 2", counts["🔥"])
	}
	applyReactionChange(counts, "🔥", "❤️") // switch
	if counts["🔥"] != 1 || counts["❤️"] != 49 {
		t.Errorf("switch: got %v", counts)
	}
	applyReactionChange(counts, "❤️", "") // withdraw
	if counts["❤️"] != 48 {
		t.Errorf("withdraw: ❤️ = %d, want 48", counts["❤️"])
	}
	solo := map[string]int{"👍": 1}
	applyReactionChange(solo, "👍", "")
	if _, ok := solo["👍"]; ok {
		t.Errorf("a count reaching zero should drop its pill: %v", solo)
	}
}

func TestNewsletterPostVMMediaOnly(t *testing.T) {
	vm := newsletterPostVM(client.NewsletterMessage{ID: "p", Attachment: &client.Attachment{Kind: "image"}}, time.Now())
	if vm.Text != "" {
		t.Fatalf("a captionless photo post has no text line, got %q", vm.Text)
	}
	if got := attachmentChipText(&client.Attachment{Kind: "video", DurationSecs: 75}); got != "🎥 Video · 1:15" {
		t.Fatalf("video chip = %q", got)
	}
	if got := attachmentChipText(&client.Attachment{Kind: "document", Filename: "a.pdf"}); got != "📎 a.pdf" {
		t.Fatalf("document chip = %q", got)
	}
	if hasCategories([]client.Newsletter{{ID: "1"}, {ID: "2"}}) || !hasCategories([]client.Newsletter{{ID: "1", Category: "News"}}) {
		t.Fatal("hasCategories")
	}
}
