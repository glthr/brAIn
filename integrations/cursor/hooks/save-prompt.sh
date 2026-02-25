#!/bin/bash
# Cursor beforeSubmitPrompt hook — saves the user's prompt so brain-encode.sh can
# encode the full conversation turn when the agent stops.
#
# Input (stdin): JSON with conversation_id, generation_id, prompt, attachments,
#                hook_event_name, workspace_roots
# Output: none (exit 0 to allow the prompt to proceed)

export PATH="$HOME/bin:$PATH"

# Hand stdin off to Python for robust JSON handling
python3 - <<'PYEOF'
import sys, json, os, subprocess, re

try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)

gen_id = d.get("generation_id", "")
prompt = d.get("prompt", "")

if not gen_id or not prompt:
    sys.exit(0)

# Ask brain who the current user is (still write file if unknown)
path_env = os.path.expanduser("~/bin") + ":" + os.environ.get("PATH", "")
env = {**os.environ, "PATH": path_env}
user_id = ""
user_name = ""
try:
    result = subprocess.run(
        ["brain", "whoami"],
        capture_output=True, text=True,
        env=env,
        timeout=5,
    )
    line = (result.stdout or "").strip()
    if line and line not in ("unknown", "user: unknown"):
        # Parse "Name (id=user-xxx interactions=N)" -> user_id, user_name
        m = re.search(r"\s*\(id=([^)\s]+)", line)
        if m:
            user_id = m.group(1)
            user_name = line[: m.start()].strip() or user_id
        else:
            user_id = line
            user_name = line
except Exception:
    pass

if not user_id:
    user_id = "unknown"
    user_name = "unknown"

os.makedirs("/tmp/cursor-brain", exist_ok=True)

tmp = f"/tmp/cursor-brain/{gen_id}.json"
with open(tmp, "w") as f:
    json.dump({"prompt": prompt, "user_id": user_id, "user_name": user_name}, f)

PYEOF

exit 0
