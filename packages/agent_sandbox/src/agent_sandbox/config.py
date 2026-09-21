"""Static configuration for the agent graph."""
import os

# Docker
AGENT_IMAGE = os.getenv("AGENT_IMAGE", "claude-agent:latest")       # if AGENT_IMAGE is not set, use the default image
CONTAINER_RUNTIME = os.getenv("CONTAINER_RUNTIME", "runsc")  # gVisor
CONTAINER_PREFIX = os.getenv("CONTAINER_PREFIX", "agent-")
CONTAINER_LABEL = "sys_dev_ecosystem.agent"

# Claude CLI inside the container
CLAUDE_BIN = os.getenv("CLAUDE_BIN", "claude")
AUTH_STATUS_CMD = [CLAUDE_BIN, "auth", "status"]
AUTH_LOGIN_CMD = [CLAUDE_BIN, "auth", "login"]
PROMPT_CMD = [CLAUDE_BIN, "-p", "--output-format", "text"]

# Loop guards
MAX_AUTH_ATTEMPTS = int(os.getenv("MAX_AUTH_ATTEMPTS", "5"))
EXIT_WORDS = {"/quit", "/exit", "quit", "exit"}

# Interactive login relay
AUTH_CODE_PROMPT = "Paste code here"           # text the CLI prints when it waits for the OAuth code
AUTH_FAIL_MARKER = "Invalid code"             # text the CLI prints on a rejected code
AUTH_TIMEOUT = float(os.getenv("AUTH_TIMEOUT", "60"))   # seconds to wait for CLI output per step
