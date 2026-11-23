# Requirement change request: restart handling in agent-net and agent-sandbox

Date: 2026-11-02. Status: open.

This file lists the SFRs and SSRs that must be updated, and the new requirements that must be written, after the restart-handling change implemented on 2026-11-02 (code in `packages/agent-net` and `packages/agent-sandbox`; both package READMEs describe it). No `.yml` file under `design/` has been changed yet. Line numbers (`L`) refer to the current `.yml` files. File names are given without the `.yml` suffix and relative to `design/03_sys_req/`.

## Changes

- **C1 Agent start.** PID 1 stays tini running dockerd (image unchanged). No agent process runs at container start. The agent runs only via `docker exec -u 1001` (enter_agent) after pause, arm, check-gate, unpause.
- **C2 Name-keyed disarm.** Works for a running, stopped or removed container. Removes the `@released` elements tagged with the container name plus MACs from the docker/port table, `as_port_<id>`, all iptables rules tagged `agent-net:<name>`, the neighbour entries of its IPs (never another live container's) and FDB entries on any bridge port. Never touches the `agents_gate` table, its chains or the `agent-net:gate` rule. Idempotent (exit 0 when nothing to do). Runs on `die` (guard), as arm step 6 (clean slate) and before `docker create`.
- **C3 Released comment.** `@released` elements carry `comment = <container name>`, added in the atomic release. New subcommands: `create_agent_net released` (lists `<mac> <comment>`) and `create_agent_net evict <mac>`.
- **C4 Guard** (`agent_lifecycle guard`).
  - Follows container start, unpause, die and update, and network connect and disconnect.
  - Holds the agent lock through check and remove.
  - On `die` it runs disarm. On the other events it runs check-gate and removes the container on a clean failure (exit 2).
  - On errors it disarms first and removes only if disarm exited 0; otherwise it leaves the container in place (best-effort pause).
  - Reconciles at start-up and every 60 s (`AGENT_SANDBOX_GUARD_RECONCILE`): check-gate every agent container whose lock is free, evict untagged elements, disarm names whose container is gone.
- **C5 Arm precondition.** Runtime `runsc` and restart policy exactly `"no"` (`""` is refused). check-gate adds a `container confinement` control: restart `"no"`, agents-net only, Running (not required unpaused). The arm pipeline is 13 steps.
- **C6 Mark.** Each per-port ingress accept rule sets mark bit `0x10000000`. `agents_gate` ingress accepts only `ether saddr @released` with the bit set, then clears it. A gate without the mark is a mismatch (fail closed).

## Decisions

- Guard removal keeps the fail-closed rule: remove only after a successful disarm; a failed disarm leaves the container in place. The rewordings of sfr-agc-016 L19, sfr-net-011 L18, ssr-agc-016 L16 and sfr-agc-008 L18 add guard removal after a successful disarm. They are not a policy reversal. sfr-agc-021 L18 (create path, failed unpause) is unchanged in meaning.
- A restarted container is removed, not re-armed.
- Blocking a deliberate `docker restart` is out of scope: it needs Docker-socket (root-equivalent) access, and root can rewrite the egress rules anyway.
- dockerd stays under tini, so ssr-img-017 needs no change.

## Affected requirements

### agent-sandbox/

| Requirement | Issue | Change |
|---|---|---|
| sfr-agc-015_locate_container_to_remove | L6 skips disarm when the container is absent | C2 |
| sfr-agc-016_disarm_network_port | L18 "vanished = failed disarm"; L19 guard leaves container in place; no comment/evict | C2, C3, C4 |
| sfr-agc-022_clean_up_partial_creation_after_start_or_pause | L15-17 "no disarm is needed"; "nothing removed after a failed create" | C1, C2, C4 |
| ssr-agc-002_no_reachability_until_armed | L15 "verified once" by check-gate; "no gate check before the start"; L16 "never attached or detached later" | C4, C5 |
| ssr-agc-016_port_closed_before_container_removed | L8 "every removal"; L16 leave in place on failed disarm; disarm now also on die | C2, C4 |
| sfr-agc-007_attach_and_arm_network | L17-18 read-back; no clean slate, no runsc precondition | C2, C4, C5 |
| sfr-agc-008_fail_closed_on_arming_failure | L18 "remove nothing" when disarm fails; guard error path must keep this | C4 |
| sfr-agc-009_verify_final_state | L16 no network attachment or restart policy check | C5 |
| sfr-agc-010_report_success | L15 fixed "[n/13]"; create pipeline is now 14 steps (new step 7 clean leftovers); L16 exit-0 conditions | C1, C2, C4 |
| sfr-agc-011_report_failure_with_distinct_status | L15 exit 1 on held lock (guard can now hold it); [n/13] | C1, C4 |
| sfr-agc-021_unpause_after_arming | L15/L17 agent exec ordering vs resolv exec | C1 |
| sfr-agc-024_write_resolv_conf_after_release | L17-18 agent work only after success report; agent exec timing | C1 |
| sfr-agc-018_verify_container_removed | L6/L19 only container absence; no leftover network-state check | C2 |
| ssr-agc-006_single_run_single_executable | L14-15 agent exec and guard reconcile are separate programs/steps | C1, C4 |
| ssr-agc-014_no_engine_restart_policy | L16 "external watchdog repeats the arming" vs guard remove-on-restart | C2, C4 |
| sfr-agc-002_refuse_duplicate_container | L15-16 pre-create disarm must run after the duplicate refusal and not touch an existing container | C2 |
| sfr-agc-006_create_and_start_container | L6/L25 idle init not stated | C1 |
| sfr-agc-017_remove_container | L6/L15 removal after die/guard disarm | C2, C4 |
| ssr-agc-001_isolated_runtime_only | L14 arm now also checks runtime | C5 |
| sfr-rtc-001_verify_isolated_runtime_registered | L14 runsc only checked before creation | C5 |

### agent-net/

| Requirement | Issue | Change |
|---|---|---|
| sfr-net-001_reject_invalid_invocation | L14 "exactly four invocation forms"; L19 arm refusals lack runsc/restart | C3, C5 |
| sfr-net-007_report_success | L15 lists arm, disarm, check-gate only | C3 |
| sfr-net-008_attach_container_to_agent_network | L15 no runsc; L21 MAC eviction vs full clean slate; L22 gate touch limits vs element removal | C2, C3, C5 |
| sfr-net-009_apply_host_side_controls | L17 neighbour "fail arm" branch vs clean slate | C2 |
| sfr-net-010_release_port_after_verification | L17 no comment, no mark; L20 check-gate between release and unpause | C1, C3, C6 |
| sfr-net-011_keep_container_gated_on_arming_failure | L14 MAC out of released vs clean slate; L18 removal left to caller | C2, C4 |
| sfr-net-012_disarm_container_port | L6 one named container; L16 missing container = exit 1; L18 MAC source; L20 window | C2, C3 |
| sfr-net-014_remove_agent_containers | L17 finds containers only by attachment; orphaned elements/tables missed | C2, C3, C4 |
| sfr-net-015_remove_leftover_port_tables | L6/L16 timing "once containers removed" | C2 |
| sfr-net-016_remove_leftover_tagged_rules | L6 name-keyed sweep must exclude `agent-net:gate` | C2 |
| sfr-net-018_verify_and_report_removal | L16 read-back omits @released, neighbour, FDB | C2 |
| sfr-net-021_check_container_port_controls | L16-17 control list; L22 "11 control lines" (now 12 + summary) | C3, C5, C6 |
| sfr-net-023_stop_when_removal_state_unreadable | L16 survey lacks @released, neighbour, FDB | C2 |
| sfr-net-024_limit_layer3_destinations | L14-19 accept rules now set the mark | C6 |
| sfr-net-025_install_and_verify_agents_net_gate | L6, L17, L22 one accept rule, "elements not checked"; L24 ordering vs restore | C3, C4, C6 |
| sfr-net-026_remove_agents_net_gate | L15 disarm must never touch gate table/rule | C2 |
| ssr-net-003_no_agent_port_open_during_removal | L6/L16 window; L15 removal order; L17 | C2 |
| ssr-net-004_removal_leaves_unrelated_state_untouched | L6 "every table other than as_port_* unchanged" vs element removal in agents_gate | C2, C3 |
| ssr-net-005_no_frame_passes_gate_for_unreleased_port | L14 one accept rule per chain; L15 add/remove only by release/disarm (evict is new) | C3, C4, C6 |

### agent-host/

| Requirement | Issue | Change |
|---|---|---|
| sfr-host-011_install_agent_tools | L20 guard unit named, behaviour undefined; L27 | C4 |
| sfr-host-009_verify_acceptance_criteria | L29 only checks guard active | C4 |
| sfr-host-013_obtain_tool_reference_hashes, ssr-host-007_installed_tools_root_owned | only if a new binary is added (none added) | none |

### agent-image/

| Requirement | Issue | Change |
|---|---|---|
| ssr-img-017_image_content_defined | no change (dockerd kept under tini) | C1 |

## New requirements to write

- Guard behaviour: events, reconcile interval, error path, order vs `agent-restore.service`, relation to the "external watchdog".
- Agent-lock scope: who holds it and for how long.
- Name-keyed disarm contract.
- Arm clean-slate contract.
- `@released` element comment format.
- `released` and `evict` subcommands.
- Arm runsc precondition.
- check-gate confinement control.
- Agent start via `docker exec` after release.
- Leftover-network-state check after removal.
- Orphan reconciliation.
- The mark: bit, set by the port table, required and cleared by the gate, mismatch = fail closed.

## Pre-existing contradictions to fix in the same pass

- sfr-net-012 L18 (released entry first) vs ssr-net-003 L15 (table first).
- ssr-net-004 L6 vs sfr-net-026 (`agents_gate` is deleted by removal).
- sfr-agc-007 L17 already requires read-back and sfr-net-008 L15 already requires restart `"no"`. C5 closes an implementation gap there rather than changing the spec.

## Open items

- Stopped containers are removed at the next reconcile (check-gate requires Running). Pending user decision: keep or skip stopped containers.
- Live agent-host evidence is pending: the `docker restart` event sequence, MAC/IP retention, and the mark on a live gate. So far the mark surviving and the element comments were observed only in an isolated namespace on 6.6.87.2-microsoft-standard-WSL2 with nftables 1.0.9.
- Existing gates without the mark must be recreated (`remove_agent_net` + `create_agent_net`; restore does it at the next dockerd start).

## Verification note

Checked against the current files: sfr-agc-016 (L18, L19), sfr-net-011 (L14, L18), sfr-net-012 (L6, L16, L18, L20), ssr-net-003 (L15), ssr-net-004 (L6), sfr-agc-010 (L15, L16). All IDs, titles and line numbers matched; no corrections were needed. The remaining entries were not opened, and the glob only confirmed that their files exist.
