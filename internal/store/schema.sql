CREATE TABLE IF NOT EXISTS chats (
    jid TEXT PRIMARY KEY,
    is_group INTEGER NOT NULL DEFAULT 0,
    name TEXT,
    pinned INTEGER NOT NULL DEFAULT 0,
    muted INTEGER NOT NULL DEFAULT 0,
    unread_count INTEGER NOT NULL DEFAULT 0,
    last_message_ts INTEGER NOT NULL DEFAULT 0
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
    PRIMARY KEY (chat_jid, msg_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON messages(chat_jid, ts);

CREATE TABLE IF NOT EXISTS reactions (
    chat_jid TEXT NOT NULL,
    msg_id TEXT NOT NULL,
    reactor_jid TEXT NOT NULL,
    emoji TEXT NOT NULL,
    ts INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (chat_jid, msg_id, reactor_jid)
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
