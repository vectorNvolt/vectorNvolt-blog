---
title: Agent dashboard — chapter overview
slug: agent-dashboard-overview
status: draft
tags: [langgraph, agent-sandbox, agent-dashboard]
custom_excerpt: "What this chapter covers: an observability dashboard for the agent-sandbox harness, and how the posts build on each other."
code_tag: agent-sandbox/v0.2.0
code_package: agent_sandbox
---

## Why this chapter

Chapter 02 built the LangGraph harness that provisions a gVisor-sandboxed container, launches Claude Code inside it, and drives the run turn by turn — but the only way to see or talk to that run today is the raw terminal the harness happens to print to. This chapter's goal is a rich, rendered view: a UI that lets an operator watch and communicate with the LangGraph app — and, through it, the Claude agent living inside the container — instead of reading a scrolling text log.
