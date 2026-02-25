# Backup and Restore

The brain file (`~/.brain/agent.brain`) is a single SQLite database. You can back it up, restore it, or move it to another machine.

## Normal backup

To create a timestamped copy in `~/.brain/backups/` and update the brain’s last-backup metadata:

```bash
brain backup
```

This writes a file like `~/.brain/backups/agent.brain.20250222-143022`. You can schedule it with cron:

```bash
# Example: daily backup at 2 AM
0 2 * * * brain backup
```

The brain records `last_backup_at` in its metadata so you can see when it was last backed up (`brain meta`).

## Safe copy for migration or manual backup

If you want to copy the file yourself (e.g. to another machine or external storage), use a single-file copy without SQLite WAL sidecars:

1. **Stop the daemon** so nothing is writing to the brain: `brain uninstall-service` or stop the daemon process.
2. **Compact the brain** so the database is one file on disk (no `-shm`/`-wal`):  
   `brain compact`  
   This requires exclusive access; close any other process using the brain file.
3. **Copy the file:**  
   `cp ~/.brain/agent.brain /destination/agent.brain`  
   (or `scp`, rsync, etc.)  
   Optionally copy `~/.brain/config` if you want the same daemon/LLM settings on the other side.
4. **Restart the daemon** on the source machine if needed: `brain install-service`.

## Restore from a backup

1. **Stop the daemon** on the machine where you are restoring: `brain uninstall-service` or stop the daemon.
2. **Replace the brain file:**  
   `cp ~/.brain/backups/agent.brain.20250222-143022 ~/.brain/agent.brain`  
   (or copy from your migration destination).
3. **Ensure the brain is initialized:**  
   `brain init`  
   This is idempotent; it won’t overwrite an existing valid brain.
4. **Start the daemon again:**  
   `brain install-service`

## Move brain to another machine

1. On the **source** machine: stop the daemon, run `brain compact`, then copy `~/.brain/agent.brain` (and optionally `~/.brain/config`) to the new machine (e.g. into `~/.brain/` there).
2. On the **destination** machine: ensure `~/.brain/` exists and contains `agent.brain`. Run `brain init` if the directory was created from scratch (idempotent). Run `brain install-service` to start the daemon. Install or configure the same LLM (e.g. Ollama) so consolidation and encoding work; see [Configuration](configuration.md) and [LLM Integration](llm.md).
