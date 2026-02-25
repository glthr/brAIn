CREATE TABLE memories (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	memory_type   TEXT    NOT NULL,
	content       TEXT    NOT NULL,
	salience      REAL    NOT NULL DEFAULT 0.5,
	retrievals    INTEGER NOT NULL DEFAULT 0,
	tags          TEXT    NOT NULL DEFAULT '[]',
	metadata      TEXT    NOT NULL DEFAULT '{}',
	user_id       TEXT    DEFAULT NULL,
	agent         TEXT    NOT NULL DEFAULT '',
	embedding     BLOB,
	created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
	last_accessed DATETIME NOT NULL DEFAULT (datetime('now')),
	expires_at    DATETIME
);

CREATE INDEX idx_memories_type      ON memories(memory_type);
CREATE INDEX idx_memories_salience  ON memories(salience);
CREATE INDEX idx_memories_created   ON memories(created_at);
CREATE INDEX idx_memories_expires   ON memories(expires_at);
CREATE INDEX idx_memories_user      ON memories(user_id);
CREATE INDEX idx_memories_agent     ON memories(agent);

CREATE TABLE contacts (
	id                TEXT PRIMARY KEY,
	entity_type       TEXT NOT NULL DEFAULT 'agent',
	name              TEXT NOT NULL DEFAULT '',
	trust             REAL NOT NULL DEFAULT 0.5,
	competence_trust  REAL NOT NULL DEFAULT 0.5,
	reliability_trust REAL NOT NULL DEFAULT 0.5,
	notes             TEXT NOT NULL DEFAULT '',
	social_schema     TEXT NOT NULL DEFAULT '',
	behavioral_priors TEXT NOT NULL DEFAULT '',
	created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
	updated_at        DATETIME NOT NULL DEFAULT (datetime('now')),
	last_analyzed_at  DATETIME
);

CREATE INDEX idx_contacts_entity_type ON contacts(entity_type);

CREATE TABLE interactions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	contact_id TEXT     NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
	type       TEXT     NOT NULL,
	outcome    TEXT     NOT NULL,
	valence    REAL     NOT NULL DEFAULT 0.0,
	notes      TEXT     NOT NULL DEFAULT '',
	metadata   TEXT     NOT NULL DEFAULT '{}',
	created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_interactions_contact ON interactions(contact_id);
CREATE INDEX idx_interactions_created ON interactions(created_at);

CREATE TABLE episodic_archive (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	content     TEXT    NOT NULL,
	tags        TEXT    NOT NULL DEFAULT '[]',
	metadata    TEXT    NOT NULL DEFAULT '{}',
	user_id     TEXT    DEFAULT NULL,
	agent       TEXT    NOT NULL DEFAULT '',
	created_at  DATETIME NOT NULL,
	archived_at DATETIME NOT NULL DEFAULT (datetime('now')),
	semantic_id INTEGER DEFAULT NULL
);

CREATE INDEX idx_archive_user    ON episodic_archive(user_id);
CREATE INDEX idx_archive_created ON episodic_archive(created_at);

CREATE TABLE consolidation_log (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	stats      TEXT     NOT NULL DEFAULT '{}',
	created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE brain_meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
