# agent-view

Read-only HTTP viewer for ngspice exec results. It runs in the `agent-host` VM as `agent-operator` (uid 1001, owner of the workspace files) and shows the exec dirs that `run_ephemeral_container` writes to `<ws>/.output/exec/<execId>/` (`result.json`, `meta.json`, `stdout.log`, `stderr.log`, `in/<name>`, `out/*`).

Go 1.22, module `agent-view`, standard library only.

## Guarantees

- Read-only: only GET and HEAD are served (anything else is 405); files are opened with `os.Open`, nothing is created or written under the workspace.
- Loopback only: `--listen` must be 127.0.0.0/8 or `::1` (else exit 2), and requests whose `Host` header is not a loopback host are rejected (421), which blocks DNS rebinding.
- Only `<ws>/.output/exec/<id>/` is exposed. Exec ids match `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`; paths are relative, without `..`, `\` or NUL, and must name a regular file inside that exec dir, else 404 (malformed id or path: 400). Every component below `--ws` is opened with `openat(O_NOFOLLOW)` from the workspace root fd (no check-then-open race with the agent), so a symlink anywhere below `--ws` is refused (404), including symlinks that stay inside the exec dir. Files above 64 MiB give 413.
- Every response carries a restrictive `Content-Security-Policy`, `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`.

## API

| Method and path | Response |
| --- | --- |
| `GET /api/execs` | `{"execs":[{"id","mtime","result","meta","inputs","outputs"}]}`, newest first; `result`/`meta` are the parsed JSON or `null` |
| `GET /api/execs/{id}/file?path=<rel>` | raw bytes of a file in the exec dir (`no-store`); text types for `.log .json .cir .txt` and `in/` |
| `GET /api/events` | Server-Sent Events: `event: execs` when an exec dir, `result.json` or `out/` file changes (polled every 1 s), `: ping` every 15 s |
| `GET /` and assets | embedded UI bundle (503 text when not built) |

## Build

```
cd packages/agent-view/web && npm install && npm run build
cd packages/agent-view && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/agent_view ./cmd/agent_view
cd packages/agent-view && go vet ./... && go test ./...
```

`npm run build` writes `internal/ui/static/app.js` and `app.css` (committed); `go:embed` picks them up at Go build time.

## Run

```
agent_view --ws /home/agent-operator/Workspace-Agents/<id> [--listen 127.0.0.1:8787]
```

Prints `agent_view: serving <ws> on http://<addr>` to stderr. Usage errors exit 2.
