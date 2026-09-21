"""Docker/gVisor container lifecycle for one Claude CLI agent."""
from __future__ import annotations

import docker
from docker.errors import NotFound
from docker.models.containers import Container

from agent_sandbox import config

_client: docker.DockerClient | None = None   # _client module-level cache which means we reuse the same Docker client across function calls
                                             # docker client is a component that manages communication with the Docker daemon, it is located
                                             # at the module level for reuse -> each module need to import and use this shared client instance,
                                             # there is only one client instance per process where process means the running instance of the Python interpreter


def client() -> docker.DockerClient:
    global _client                           # "global" keyword indicates that we are referring to the module-level _client variable, not a local one
    if _client is None:
        _client = docker.from_env()          # docker.from_env() creates a Docker client based on environment variables, allowing communication with the Docker daemon
                                             # these are the env variable what is used: DOCKER_HOST, DOCKER_TLS_VERIFY, DOCKER_CERT_PATH
                                             # DOCKER_HOST specifies the URL to the Docker daemon.
                                             # DOCKER_TLS_VERIFY enables or disables TLS verification.
                                             # DOCKER_CERT_PATH specifies the path to TLS certificates.
    return _client


def container_name(agent_name: str) -> str:            # Define the naming convention for containers associated with agents.
    return f"{config.CONTAINER_PREFIX}{agent_name}"    # Construct the container name by prefixing the agent name with the configured container prefix.
                                                       # CONTAIENER_PREFIX is a prefix string defined in the configuration to standardize container names.

def find(agent_name: str) -> Container | None:
    try:                                                                # "try-except" block is used since the container may not exist, and attempting to
                                                                        # get a non-existent container raises a NotFound exception. Which will be caught by
                                                                        # the except block below.
        return client().containers.get(container_name(agent_name))
    except NotFound:
        return None


def create(agent_name: str) -> Container:
    """Create and start a gVisor-sandboxed container for the agent."""
    c = client().containers.run(
        image=config.AGENT_IMAGE,                                           # AGENT_IMAGE is defined in the configuration and specifies the Docker image to use for the agent container.
        name=container_name(agent_name),
        runtime=config.CONTAINER_RUNTIME,                                   # CONTAINER_RUNTIME specifies the runtime to use for the container, e.g., runc or gVisor.
        detach=True,
        labels={config.CONTAINER_LABEL: agent_name},
        network_mode="bridge",
        read_only=False,
        security_opt=["no-new-privileges"],
        cap_drop=["ALL"],
    )
    return c


def ensure_running(c: Container) -> Container:
    c.reload()
    if c.status != "running":
        c.start()
        c.reload()
    return c


def exec(c: Container, cmd: list[str], stdin: str | None = None, timeout: int | None = None) -> tuple[int, str]:
    """Run a command inside the container; returns (exit_code, combined output)."""
    # docker-py has no stdin-with-exec_run; shell-pipe the input in when given.
    if stdin is not None:
        import shlex
        cmd = ["sh", "-c", f"printf %s {shlex.quote(stdin)} | {' '.join(shlex.quote(a) for a in cmd)}"]     # merge stdin into the command via a shell pipe, stdin is at the beginning of the command's input since it is piped into the command.
    code, out = c.exec_run(cmd, demux=False)
    return code, (out or b"").decode(errors="replace")


# --- interactive exec sessions ----------------------------------------------
# `claude auth login` must receive the OAuth code in the *same* process that printed
# the URL (PKCE), and it blocks until then. So the login runs as a long-lived exec
# with an attached socket that survives across graph nodes / interrupts.
# - PKCE (Proof Key for Code Exchange) is used in the OAuth flow to enhance security by
# mitigating authorization code interception attacks.
import socket
import struct
import time

# Exec sessions for interactive command execution inside containers. Interactivity is achieved through maintaining an open socket connection for stdin and stdout.
# - tty is needed for interactive sessions to allocate a pseudo-TTY inside the container. Without it the interactive commands may not behave correctly as they would
# in a normal terminal since the pseudo-TTY provides the necessary terminal emulation for interactive commands to function correctly.
# - Terminal here is emulated by the pseudo-TTY allocated inside the container. Terminal then will use bash or the default shell inside the container for command execution,
# this should be defined by the container's environment and configuration or by explicitly specifying the shell to be used for command execution e.g. `cmd=["/bin/bash"]`.
# - the communication chain look like this operator->host terminal->stdin/stdout socket->docker exec instance->container process
# - docker exec instance is directly connected to the container process and on the other side it communicates with the host terminal through the stdin/stdout socket (via inheritance during process fork).
# the mechanism to connect stdin/stdout of the host terminal to the container process involves creating an exec instance with a TTY and stdin enabled,
# starting the exec instance with a socket, and then using this socket to relay input and output between the host terminal and the container process.
class LineTooLong(Exception):
    """The process wrote more than `max_line` bytes without a newline (or one Docker
    frame larger than that). The caller closes the session; nothing is read further."""


class ExecSession:
    def __init__(self, c: Container, cmd: list[str], tty: bool = True, max_line: int | None = None):
        self._api = client().api
        self._tty = tty
        self._max_line = max_line or config.CHAT_MAX_LINE
        # Non-TTY mode: Docker multiplexes stdout/stderr on the socket with 8-byte frame headers,
        # which `readline` demuxes. Used for NDJSON protocols where a TTY would echo stdin and
        # CR-translate output.
        self._raw = b""            # undecoded bytes (partial frame)
        self._stdout = ""          # decoded stdout awaiting a newline
        self.stderr = ""
        self.exec_id = self._api.exec_create(c.id, cmd, stdin=True, tty=tty)["Id"]   # Create an exec instance in the container with a TTY and stdin enabled, and store its ID.
                                                                                     # "exec instance" is a running command within the container that can be attached to for input/output.
                                                                                     # exec instance under the hood is a process running inside the container, which can be controlled and communicated with through the Docker API.
                                                                                     # this command running means the exec instance is active and can be interacted with through the attached socket.
                                                                                     # it is different from simple command execution because it allows interactive communication with the running process.
                                                                                     # interactive comm is possible due to the fact that the exec instance maintains an open socket connection for stdin and stdout.
                                                                                     # exec instance is represented by the `exec_id` and can be interacted with using the attached socket.
                                                                                     # "cmd" is the command that will be executed inside the container within this exec instance.
        sio = self._api.exec_start(self.exec_id, socket=True, tty=tty)
        self._sock: socket.socket = getattr(sio, "_sock", sio)                       # ":" defines the type hint for the `_sock` attribute as a `socket.socket` instance.
                                                                                     # then "=" assigns the actual socket object to the `_sock` attribute.
                                                                                     # getattr(): retrieves the `_sock` attribute from `sio` if it exists; otherwise, it returns `sio` itself.
        self._sock.settimeout(1)

    def read_until(self, marker: str | None, timeout: float) -> str:                    # Read from the socket until the specified marker is found, EOF is reached, or the timeout elapses.
        """Read output until `marker` appears, EOF, or `timeout` seconds elapse."""
        buf, deadline = "", time.monotonic() + timeout                                  # "deadline" is the absolute time at which the read operation should stop, calculated as the current time plus the specified timeout.
                                                                                        # "" after deadline indicates that the buffer is initially empty before any data is read from the socket.
                                                                                        # this assignment is initializing the buf and deadline by setting the buffer to an empty string and calculating the absolute deadline time for the read operation.
        while time.monotonic() < deadline:                                              # Loop until the current time exceeds the deadline, checking for incoming data from the socket.
                                                                                        # As much data arrived within this iteration as possible is read from the socket, buf accumulates it upto the point where either the marker is found, EOF is reached, or the timeout occurs. no memory limit on the buffer is enforced.
            try:
                chunk = self._sock.recv(4096)                                           # Receive up to 4096 bytes of data from the socket then continue processing it.
            except socket.timeout:
                continue
            if not chunk:
                break
            buf += chunk.decode(errors="replace")
            if marker and marker in buf:
                break
            if len(buf) > self._max_line:                                           # same bound as readline(): the login relay must not grow the host buffer without limit either
                raise LineTooLong(f"more than {self._max_line} bytes without the marker")
        return buf

    def readline(self, timeout: float) -> str | None:
        """Next newline-terminated stdout line (non-TTY sessions).
        Returns None on timeout, "" on EOF. stderr frames accumulate in `self.stderr`.
        Raises LineTooLong once `max_line` bytes are buffered without a newline — a
        line is a JSON event, and an event that big is exhaustion, not a message."""
        deadline = time.monotonic() + timeout
        while True:
            nl = self._stdout.find("\n")
            if nl > self._max_line or (nl < 0 and len(self._stdout) > self._max_line):
                raise LineTooLong(f"line over {self._max_line} bytes")
            if nl >= 0:
                line, self._stdout = self._stdout[:nl], self._stdout[nl + 1:]
                return line
            if time.monotonic() >= deadline:
                return None
            try:
                chunk = self._sock.recv(65536)
            except socket.timeout:
                continue
            if not chunk:
                return ""
            self._raw += chunk
            self._demux()

    def _demux(self) -> None:
        if self._tty:
            self._stdout += self._raw.decode(errors="replace")
            self._raw = b""
            return
        while len(self._raw) >= 8:
            stream, size = struct.unpack(">BxxxL", self._raw[:8])
            if size > self._max_line:                                       # the header is written by the daemon, but the payload it announces would be buffered whole
                raise LineTooLong(f"frame of {size} bytes")
            if len(self._raw) < 8 + size:
                return
            payload, self._raw = self._raw[8:8 + size], self._raw[8 + size:]
            if stream == 2:
                self.stderr += payload.decode(errors="replace")
            else:
                self._stdout += payload.decode(errors="replace")

    def write(self, text: str) -> None:
        self._sock.sendall(text.encode())

    def close_stdin(self) -> None:
        """Send EOF on the process's stdin (what `docker exec` does on CloseWrite)."""
        try:
            self._sock.shutdown(socket.SHUT_WR)
        except OSError:
            pass

    def exit_code(self) -> int | None:
        return self._api.exec_inspect(self.exec_id)["ExitCode"]

    def close(self) -> None:
        try:
            self._sock.close()
        except OSError:
            pass


_sessions: dict[str, ExecSession] = {}                          # _sessions is a dictionary mapping container IDs to their corresponding ExecSession instances.
                                                                # "str" represents the container ID as a string.

def open_session(c: Container, cmd: list[str], tty: bool = True) -> ExecSession:
    close_session(c)                                            # Close any existing exec session for the container before opening a new one.
    _sessions[c.id] = ExecSession(c, cmd, tty=tty)              # Create a new exec session for the container and store it in the _sessions dictionary.
                                                                # _sessions[c.id] address by key=container ID, value=ExecSession(c, cmd)
    return _sessions[c.id]                                      # Return the newly created exec session.


def get_session(c: Container) -> ExecSession | None:
    return _sessions.get(c.id)


def close_session(c: Container) -> None:
    s = _sessions.pop(c.id, None)
    if s:
        s.close()
