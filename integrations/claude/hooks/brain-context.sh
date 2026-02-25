#!/bin/bash
# Claude Code UserPromptSubmit hook — auto-loads brain context before every turn.
#
# Input (stdin): JSON with session_id, transcript_path, cwd, permission_mode,
#                hook_event_name, prompt
# Output: brain context printed to stdout (injected as context by Claude Code)

export PATH="$HOME/bin:$PATH"

python3 - <<'PYEOF'
import sys, json, os, subprocess, re

try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)

brain = os.path.expanduser("~/bin/brain")
env   = {**os.environ, "PATH": os.path.expanduser("~/bin") + ":" + os.environ.get("PATH", "")}

# Extract up to 3 keywords from the prompt for targeted recall
prompt = d.get("prompt", "")
words  = [w.strip(".,!?;:'\"()[]") for w in prompt.split() if len(w.strip(".,!?;:'\"()[]")) > 3]
topics = ",".join(dict.fromkeys(words[:3]))  # deduplicated, first 3

cmd = [brain, "context"]
if topics:
    cmd += ["--topics", topics]

try:
    result = subprocess.run(cmd, capture_output=True, text=True, env=env, timeout=15)
    if result.stdout:
        sys.stdout.write(result.stdout)
except Exception:
    pass

PYEOF

exit 0
