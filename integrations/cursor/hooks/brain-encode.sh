#!/bin/bash
# Cursor stop hook — encodes the completed conversation turn into the brain.
#
# Input (stdin): JSON with conversation_id, generation_id, status,
#                hook_event_name, workspace_roots
# Output: none (exit 0; stop hooks are informational only)

export PATH="$HOME/bin:$PATH"

python3 - <<'PYEOF'
import sys, json, os, subprocess, glob, time, shutil

try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)

gen_id  = d.get("generation_id", "")
status  = d.get("status", "")

# Only encode on successful completion
if status != "completed" or not gen_id:
    sys.exit(0)

tmp = f"/tmp/cursor-brain/{gen_id}.json"
if not os.path.exists(tmp):
    sys.exit(0)

try:
    with open(tmp) as f:
        saved = json.load(f)
    os.remove(tmp)
except Exception:
    sys.exit(0)

prompt    = saved.get("prompt", "")
user_id   = saved.get("user_id", "")
user_name = saved.get("user_name", "") or user_id

if not prompt or not user_id:
    sys.exit(0)

# Clean up stale temp files older than 1 hour
for stale in glob.glob("/tmp/cursor-brain/*.json"):
    try:
        if time.time() - os.path.getmtime(stale) > 3600:
            os.remove(stale)
    except Exception:
        pass

# Find brain: PATH first (including ~/bin), then fallback to ~/bin/brain
path_env = os.path.expanduser("~/bin") + ":" + os.environ.get("PATH", "")
env = {**os.environ, "PATH": path_env}
brain = shutil.which("brain", path=path_env) if hasattr(shutil, "which") else None
if not brain:
    brain = os.path.expanduser("~/bin/brain")
if not brain or not os.path.isfile(brain):
    sys.exit(0)

subprocess.run(
    [
        brain, "encode",
        "--user",     user_id,
        "--name",     user_name,
        "--agent",    "cursor",
        "--prompt",   prompt,
        "--response", "(response captured via Cursor stop hook — full transcript in Cursor)",
    ],
    capture_output=True,
    env=env,
    timeout=30,
)

PYEOF

exit 0
