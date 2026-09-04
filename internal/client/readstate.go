package client

import "go.mau.fi/whatsmeow/types"

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

// readBatch is one read receipt's worth of message ids: those in a group
// chat sent by Sender, which the receipt has to name.
type readBatch struct {
	Sender string
	MsgIDs []string
}

// readBatches splits msgIDs by the sender senderOf reports for each, in
// first-seen order. An id with no known sender (a row whose sender was
// never learned) is dropped: WhatsApp rejects a group receipt without one.
func readBatches(msgIDs []string, senderOf func(id string) string) []readBatch {
	var out []readBatch
	index := map[string]int{}
	for _, id := range msgIDs {
		sender := senderOf(id)
		if sender == "" {
			continue
		}
		i, ok := index[sender]
		if !ok {
			i = len(out)
			index[sender] = i
			out = append(out, readBatch{Sender: sender})
		}
		out[i].MsgIDs = append(out[i].MsgIDs, id)
	}
	return out
}

// receiptTarget is where a read receipt for a message from sender in chat
// is addressed. A group receipt goes to the group. A DM's messages are
// filed under the contact's phone number whichever way they came in, but
// a receipt has to go back the way the message came: to the sender's LID
// when that is what the message was addressed from.
func receiptTarget(chat, sender types.JID) types.JID {
	if chat.Server != types.GroupServer && sender.Server == types.HiddenUserServer {
		return sender
	}
	return chat
}
