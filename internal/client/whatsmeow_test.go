package client

import "testing"

func TestMentionedUsersDistinctInOrder(t *testing.T) {
	got := mentionedUsers("@554899010873 and @257157073207386, again @554899010873; mail me@x.org")
	want := []string{"554899010873", "257157073207386"}
	if len(got) != len(want) {
		t.Fatalf("mentionedUsers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := mentionedUsers("no mentions @ all @12"); got != nil {
		t.Errorf("expected none, got %v", got)
	}
}
