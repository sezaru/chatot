CREATE TABLE IF NOT EXISTS chats (
    jid TEXT PRIMARY KEY,
    is_group INTEGER NOT NULL DEFAULT 0,
    name TEXT,
    pinned INTEGER NOT NULL DEFAULT 0,
    muted INTEGER NOT NULL DEFAULT 0,
    unread_count INTEGER NOT NULL DEFAULT 0,
    last_message_ts INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS contacts (
    jid TEXT PRIMARY KEY,
    business_name TEXT,
    full_name TEXT,
    push_name TEXT,
    system_name TEXT
);

CREATE TABLE IF NOT EXISTS groups (
    jid TEXT PRIMARY KEY,
    name TEXT,
    is_parent INTEGER NOT NULL DEFAULT 0,
    linked_parent_jid TEXT
);

CREATE TABLE IF NOT EXISTS messages (
    chat_jid TEXT NOT NULL,
    msg_id TEXT NOT NULL,
    from_jid TEXT,
    from_me INTEGER NOT NULL DEFAULT 0,
    text TEXT,
    ts INTEGER NOT NULL DEFAULT 0,
    reply_to_msg_id TEXT,
    -- kind='' is a plain text/media message; a non-empty kind (e.g.
    -- 'location') marks a rich message whose body lives in payload as opaque
    -- JSON that only package client understands. The store never parses it.
    kind TEXT NOT NULL DEFAULT '',
    payload TEXT,
    -- 1 once a MESSAGE_EDIT for this id has been applied; sticky (a later
    -- non-edit re-upsert of the same row never clears it, see UpsertMessage).
    edited INTEGER NOT NULL DEFAULT 0,
    -- 1 once a REVOKE for this id has been applied; sticky like edited, so a
    -- redelivery of the original message afterwards can't un-delete it.
    deleted INTEGER NOT NULL DEFAULT 0,
    -- Outgoing delivery/read state for our own messages: 0=sent, 1=delivered,
    -- 2=read. Monotonic (see SetMessagesStatus) so a late delivered receipt
    -- after a read one can't downgrade it. Meaningless for inbound messages.
    status INTEGER NOT NULL DEFAULT 0,
    -- 1 once the message has been starred via app-state; excluded from
    -- UpsertMessage's ON CONFLICT SET (see SetMessageStarred) so a
    -- re-delivery of the original message can't unstar it.
    starred INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (chat_jid, msg_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON messages(chat_jid, ts);

-- External-content fts5 index over message text: messages has no INTEGER
-- PRIMARY KEY, so sqlite's implicit rowid (stable, unique per row) is used
-- as content_rowid. Kept in sync by triggers below; db.go backfills rows
-- written before this table existed.
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    text,
    content='messages',
    content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
END;

CREATE TABLE IF NOT EXISTS reactions (
    chat_jid TEXT NOT NULL,
    msg_id TEXT NOT NULL,
    reactor_jid TEXT NOT NULL,
    emoji TEXT NOT NULL,
    ts INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (chat_jid, msg_id, reactor_jid)
);

-- poll_votes holds decrypted poll votes: one row per (voter, selected
-- option). option_hash is the SHA-256 of the option name (WhatsApp transmits
-- votes as hashes); the store persists the raw bytes without interpreting
-- them. A voter picking N options makes N rows; re-voting replaces the set.
CREATE TABLE IF NOT EXISTS poll_votes (
    chat_jid TEXT NOT NULL,
    poll_msg_id TEXT NOT NULL,
    voter_jid TEXT NOT NULL,
    option_hash BLOB NOT NULL,
    PRIMARY KEY (chat_jid, poll_msg_id, voter_jid, option_hash)
);

CREATE TABLE IF NOT EXISTS labels (
    label_id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    color INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS label_chats (
    label_id TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    PRIMARY KEY (label_id, chat_jid)
);

CREATE TABLE IF NOT EXISTS media (
    chat_jid TEXT NOT NULL,
    msg_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    filename TEXT,
    caption TEXT,
    mime_type TEXT,
    local_path TEXT,
    -- proto.Marshal of the specific waE2E.*Message (ImageMessage etc.) that
    -- carried this attachment; unmarshalled back on download to reconstruct
    -- the whatsmeow.DownloadableMessage client.Download needs to decrypt it.
    proto_blob BLOB,
    PRIMARY KEY (chat_jid, msg_id)
);
