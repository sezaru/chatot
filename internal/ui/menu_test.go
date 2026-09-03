package ui

import "testing"

func TestMenuItemsRender(t *testing.T) {
	t.Run("separators are not actionable items", func(t *testing.T) {
		sep := menuSeparator()
		if !sep.Separator {
			t.Error("menuSeparator() should be flagged as a separator")
		}
		if sep.Label != "" || sep.Icon != "" || sep.OnActivate != nil {
			t.Error("a separator carries no label, icon or action")
		}
	})

	t.Run("labels lists only the actionable rows", func(t *testing.T) {
		items := []menuItem{
			{Icon: "📂", Label: "Archived"},
			menuSeparator(),
			{Icon: "⚙", Label: "Preferences", Accel: "Ctrl+,"},
		}
		got := menuItemLabels(items)
		want := []string{"Archived", "Preferences"}
		if len(got) != len(want) {
			t.Fatalf("menuItemLabels() = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("menuItemLabels()[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})
}

func TestMenuItemCSSClasses(t *testing.T) {
	cases := []struct {
		name string
		item menuItem
		want []string
	}{
		{"plain", menuItem{Label: "Reply"}, []string{"chatot-menu-item"}},
		{"destructive", menuItem{Label: "Delete", Destructive: true}, []string{"chatot-menu-item", "chatot-menu-item-danger"}},
		{"dim heading", menuItem{Label: "Lists", Dim: true}, []string{"chatot-menu-item", "chatot-menu-item-dim"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := menuItemCSSClasses(tc.item)
			if len(got) != len(tc.want) {
				t.Fatalf("menuItemCSSClasses() = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("class[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestWithoutMenuItem(t *testing.T) {
	items := []menuItem{
		{Label: "Reply"},
		{Label: "Edit message"},
		menuSeparator(),
		{Label: "Delete message"},
	}
	got := menuItemLabels(withoutMenuItem(items, "Edit message"))
	want := []string{"Reply", "Delete message"}
	if len(got) != len(want) {
		t.Fatalf("withoutMenuItem() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	t.Run("separators survive", func(t *testing.T) {
		if n := len(withoutMenuItem(items, "Edit message")); n != 3 {
			t.Errorf("kept %d rows, want 3 (two items and the separator)", n)
		}
	})
}

func TestLabelMenuItems(t *testing.T) {
	rows := []labelMenuRow{
		{ID: "l1", Name: "Work", Color: "#5a7ab5", Count: 3},
		{ID: "l2", Name: "Family", Color: "#c26b5c"},
	}
	items := labelMenuItems(rows, nil, nil)

	got := menuItemLabels(items)
	want := []string{"Work", "Family", "Manage lists…"}
	if len(got) != len(want) {
		t.Fatalf("labelMenuItems() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if items[0].DotColor != "#5a7ab5" {
		t.Errorf("Work dot = %q, want #5a7ab5", items[0].DotColor)
	}
	if items[0].Count != "3" {
		t.Errorf("Work count = %q, want 3", items[0].Count)
	}
	if items[1].Count != "" {
		t.Errorf("a zero count should render nothing, got %q", items[1].Count)
	}
	if items[len(items)-1].Icon != "＋" {
		t.Errorf("last row icon = %q, want ＋", items[len(items)-1].Icon)
	}

	t.Run("empty list still offers Manage lists", func(t *testing.T) {
		if got := menuItemLabels(labelMenuItems(nil, nil, nil)); len(got) != 1 || got[0] != "Manage lists…" {
			t.Errorf("labelMenuItems(nil) = %v", got)
		}
	})
}
