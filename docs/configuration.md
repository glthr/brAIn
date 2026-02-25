# Configuration

The CLI and daemon read settings from `~/.brain/config` and environment variables. The daemon creates this file with defaults when you run `brain init` or `brain install-service`.

## LLM (required for daemon)

LLM connection is required for daemon and for CLI operations that use the LLM (consolidation, encoding, profile analysis). See [LLM Integration](llm.md) and [Daemon](daemon.md) for:

- `llm_url`, `llm_model`, `llm_api_key` in `~/.brain/config`
- `BRAIN_LLM_URL`, `BRAIN_LLM_MODEL`, `BRAIN_LLM_API_KEY` environment variables

## Daemon options

See [Daemon](daemon.md) for:

- `consolidation_interval`, `profile_interval`
- `source_path`, `auto_update_on_start` (optional)
- Log level and log path

## Brain metadata (description, backup)

- **Description:** Set a free-form description with `brain meta description "..."` or when creating the brain with `brain init --desc "Your description"`. Shown in `brain meta` and helps identify the brain when moving or cloning it.
- **Backup timestamp:** The brain records when it was last backed up in `brain_meta.last_backup_at`. Run `brain backup` to copy the brain file to a timestamped backup and update this timestamp. For a manual copy without the CLI, run `brain compact` first (so the file is a single copy), then copy the file and call the API `RecordBackup` if you need the timestamp set.

## Compacting the brain file

The CLI command `brain compact` runs a WAL checkpoint so the `.brain` file is a single file on disk (no `-shm`/`-wal` sidecars). Useful before backing up or copying the brain. Requires exclusive access — close the daemon or stop other processes using the brain file first.

## Logging

The CLI logs to `~/.brain/brain.log`. The daemon logs to `~/.brain/daemon.log`. Log level and rotation are described in [Daemon](daemon.md).
