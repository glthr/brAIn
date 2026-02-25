#!/bin/bash
# Claude Code Stop hook — encodes the completed conversation turn into the brain.
#
# Input (stdin): JSON with session_id, transcript_path, cwd, hook_event_name,
#                stop_hook_active, last_assistant_message
# Runs async so it does not delay the UI.

export PATH="$HOME/bin:$PATH"

python3 - <<'PYEOF'
import sys, json, os, subprocess, re

try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)

response        = d.get("last_assistant_message", "")
transcript_path = d.get("transcript_path", "")

if not response:
    sys.exit(0)

brain = os.path.expanduser("~/bin/brain")
env   = {**os.environ, "PATH": os.path.expanduser("~/bin") + ":" + os.environ.get("PATH", "")}

# Resolve current user
try:
    r       = subprocess.run([brain, "whoami"], capture_output=True, text=True, env=env, timeout=5)
    user_id = r.stdout.strip()
except Exception:
    user_id = ""

if not user_id or "unknown" in user_id:
    sys.exit(0)

# Parse "Guillaume (id=user-guillaume interactions=N)" → "user-guillaume"
m = re.search(r"id=([^\s)]+)", user_id)
if m:
    user_id = m.group(1)

# Extract the last user prompt from the JSONL transcript
prompt = ""
if transcript_path and os.path.exists(transcript_path):
    try:
        with open(transcript_path) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    entry = json.loads(line)
                    if entry.get("type") == "user":
                        msg     = entry.get("message", {})
                        content = msg.get("content", "")
                        if isinstance(content, str):
                            prompt = content
                        elif isinstance(content, list):
                            for block in content:
                                if isinstance(block, dict) and block.get("type") == "text":
                                    prompt = block.get("text", "")
                                    break
                except Exception:
                    continue
    except Exception:
        pass

if not prompt:
    sys.exit(0)

subprocess.run(
    [
        brain, "encode",
        "--user",     user_id,
        "--agent",    "claude-code",
        "--prompt",   prompt,
        "--response", response,
    ],
    capture_output=True,
    env=env,
    timeout=30,
)

PYEOF

exit 0
