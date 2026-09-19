---
name: sandbox
description: Run untrusted or freshly generated code inside a Firecracker microVM via the sandboxd MCP tools (sandbox_run, sandbox_put_file, sandbox_create, sandbox_destroy). Use whenever you are about to execute a script you just wrote, install packages, run a test suite, or try a command whose effects you cannot predict, instead of running it on the host with Bash.
---

# Sandbox

You have MCP tools backed by sandboxd, a Firecracker microVM service. Each sandbox is a real VM with its own kernel, restored from a snapshot in ~100 ms, with deny-by-default egress. Nothing you run there can touch the host.

## When to use it

Reach for the sandbox instead of Bash when:

- you are about to run code you generated in this conversation
- the command installs packages, compiles, or runs a test suite
- the user asks to "try", "test", or "check" something and the outcome is uncertain
- the command could modify or delete files, spawn long-running processes, or make network calls

Use Bash on the host only for reading the user's repo, git, and commands the user explicitly asked to run locally.

## How to use it

1. `sandbox_put_file` to write scripts and inputs under `/work`. Parent directories are created.
2. `sandbox_run` with a shell command. It runs `sh -c <cmd>` as root, cwd `/work`. Returns `stdout`, `stderr`, `exit_code`. Default timeout 60 s; pass `timeout_ms` for longer jobs.
3. The first call lazily creates a session sandbox. Files under `/work` persist across calls for about 15 minutes. You do not need `sandbox_create` unless you want two isolated environments at once.
4. `sandbox_destroy` when you are done with an extra sandbox. The session sandbox cleans itself up.

## What is inside

Debian bookworm, `python3`, `pip`, `node` 18, `npm`, `git`, `curl`. 1 vCPU, 512 MB RAM, 2 GB disk. No systemd, no cron, no GPU.

## Network

Egress is an IP allowlist set by the operator. By default only DNS (1.1.1.1) is reachable, so `pip install` and `npm install` will fail unless the operator opened the registries. If a network call fails with a timeout, say so and ask whether egress should be widened rather than retrying.

## Reporting

Show the user the exact command and its `stdout`/`stderr`. Treat a non-zero `exit_code` as a failure to explain, not to hide. Output over a few KB should be summarized with the key lines quoted.
