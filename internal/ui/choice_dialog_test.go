package ui

import (
	"reflect"
	"testing"
)

func TestDisappearingChoices(t *testing.T) {
	opts := disappearingChoices(0)

	var labels []string
	for _, o := range opts {
		labels = append(labels, o.Label)
	}
	want := []string{"Off", "24 hours", "7 days", "90 days"}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("disappearingChoices() labels = %v, want %v", labels, want)
	}

	t.Run("a zero timer marks Off as current", func(t *testing.T) {
		if !opts[0].Current {
			t.Error("Off should be current when the timer is 0")
		}
		for _, o := range opts[1:] {
			if o.Current {
				t.Errorf("%q should not be current", o.Label)
			}
		}
	})

	t.Run("the matching duration is current", func(t *testing.T) {
		week := disappearingChoices(7 * 24 * 60 * 60)
		if !week[2].Current {
			t.Error("7 days should be current for a 604800s timer")
		}
		if week[0].Current {
			t.Error("Off should not be current for a 604800s timer")
		}
	})

	t.Run("an unrecognised timer marks nothing current", func(t *testing.T) {
		for _, o := range disappearingChoices(1234) {
			if o.Current {
				t.Errorf("%q should not be current for an unknown timer", o.Label)
			}
		}
	})

	t.Run("seconds map to the labels", func(t *testing.T) {
		got := map[string]int64{}
		for _, o := range disappearingChoices(0) {
			got[o.Label] = o.Seconds
		}
		want := map[string]int64{"Off": 0, "24 hours": 86400, "7 days": 604800, "90 days": 7776000}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("seconds = %v, want %v", got, want)
		}
	})
}

func TestBlockConfirmText(t *testing.T) {
	title, body := blockConfirmText("Alex Rivera")
	if title != "Block Alex Rivera?" {
		t.Errorf("title = %q, want Block Alex Rivera?", title)
	}
	if body == "" {
		t.Error("body should explain what blocking does")
	}

	t.Run("a nameless chat still reads sensibly", func(t *testing.T) {
		if got, _ := blockConfirmText(""); got != "Block this contact?" {
			t.Errorf("title = %q, want Block this contact?", got)
		}
	})
}
