# SQLite as the Backbone

All tables live in one SQLite database running in WAL mode:

| Table | Role |
|---|---|
| `memories` | Unified store for all memory types (discriminated by `memory_type`), with `agent` column tracking which AI created each memory and `embedding` column for semantic similarity search |
| `contacts` | Known contacts (agents **and** humans), discriminated by `entity_type`; agents have trust, humans have person schema and behavioral priors |
| `interactions` | Full interaction log (valence used for agents only; not stored for humans) |
| `consolidation_log` | Audit trail of every consolidation cycle |
| `brain_meta` | Key-value store for brain identity, model info, and backup timestamps |
| `schema_migrations` | Tracks which schema migrations have been applied |

The schema uses an embedded migration system (`internal/store/migrations/`). Each migration is a numbered SQL file applied in order. The `schema_migrations` table tracks which versions have been applied, enabling forward-compatible schema evolution.

**Adding a migration:** Add a new file under `internal/store/migrations/` with the next sequential number and a `.sql` suffix (e.g. `002_add_foo.sql`). Use idempotent SQL where possible (e.g. `CREATE TABLE IF NOT EXISTS`). Then bump the `schemaVersion` constant in `internal/store/schema.go` so the new migration is applied on the next init or open.

The `memories` table has indexes on `memory_type`, `salience`, `created_at`, `expires_at`, `user_id`, and `agent` for efficient queries across all memory subsystems. The `embedding` column stores vector embeddings (as BLOBs) for semantic similarity search when an embedding model is configured.

The `contacts` table has an `entity_type` column (`"agent"` or `"human"`) so the brain can maintain separate listings; trust is computed only for agents, while interaction and profile infrastructure is shared. An index on `entity_type` makes type-filtered queries (`ListAgents`, `ListHumans`) fast even with thousands of contacts. The `interactions` table references `contacts(id)` with foreign keys and `ON DELETE CASCADE` -- when a contact is removed, all associated data is automatically cleaned up.
