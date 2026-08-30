package store

import "strings"

// SetPollVotes replaces a voter's selections on a poll with optionHashes, in a
// single transaction (a re-vote deletes the old set before inserting the new).
// An empty optionHashes clears the voter's votes.
func (s *Store) SetPollVotes(chatJID, pollMsgID, voterJID string, optionHashes [][]byte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM poll_votes WHERE chat_jid = ? AND poll_msg_id = ? AND voter_jid = ?
	`, chatJID, pollMsgID, voterJID); err != nil {
		return err
	}
	for _, h := range optionHashes {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO poll_votes(chat_jid, poll_msg_id, voter_jid, option_hash)
			VALUES (?, ?, ?, ?)
		`, chatJID, pollMsgID, voterJID, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// pollVotesFor returns the raw votes cast on each of msgIDs (poll messages) in
// chatJID, keyed by poll message id. Mirrors reactionsFor: the store returns
// unaggregated rows and package client computes per-option counts by matching
// the hashes against the poll's option names.
func (s *Store) pollVotesFor(chatJID string, msgIDs []string) (map[string][]PollVoteRow, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(msgIDs)), ",")
	args := make([]any, 0, len(msgIDs)+1)
	args = append(args, chatJID)
	for _, id := range msgIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(`
		SELECT poll_msg_id, voter_jid, option_hash FROM poll_votes
		WHERE chat_jid = ? AND poll_msg_id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]PollVoteRow)
	for rows.Next() {
		var pollID, voter string
		var hash []byte
		if err := rows.Scan(&pollID, &voter, &hash); err != nil {
			return nil, err
		}
		out[pollID] = append(out[pollID], PollVoteRow{VoterJID: voter, OptionHash: hash})
	}
	return out, rows.Err()
}
