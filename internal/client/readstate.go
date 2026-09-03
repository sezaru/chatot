package client

// groupReadStatus is a group message's status from who has read it:
// MessageStatusRead once every participant other than the account itself
// is among the readers, MessageStatusDelivered otherwise. canon maps a JID
// to the person behind it, so a reader under a LID matches a member listed
// by phone number. Without a known membership nothing is claimed read.
func groupReadStatus(readers, participants, own []string, canon func(string) string) int {
	read := make(map[string]bool, len(readers))
	for _, r := range readers {
		read[canon(r)] = true
	}
	self := make(map[string]bool, len(own))
	for _, o := range own {
		self[canon(o)] = true
	}
	others := 0
	for _, p := range participants {
		user := canon(p)
		if self[user] {
			continue
		}
		others++
		if !read[user] {
			return MessageStatusDelivered
		}
	}
	if others == 0 {
		return MessageStatusDelivered
	}
	return MessageStatusRead
}

// readCoversChat tells whether a markChatAsRead action whose range ends at
// rangeTS (seconds; 0 when the action carries no range) accounts for a chat
// whose newest message is at lastTS: a read from before newer messages
// arrived says nothing about those.
func readCoversChat(rangeTS, lastTS int64) bool {
	return rangeTS == 0 || lastTS <= rangeTS
}

// jidUserIn tells whether jid's user part (device suffix ignored) is one
// of users.
func jidUserIn(jid string, users []string) bool {
	user := jidUser(jid)
	if user == "" {
		return false
	}
	for _, u := range users {
		if u == user {
			return true
		}
	}
	return false
}
