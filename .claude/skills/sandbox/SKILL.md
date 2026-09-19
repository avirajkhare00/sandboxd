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

## Limitations, and what to do about them

Check these before you start, and tell the user plainly when a task hits one instead of retrying or working around it on the host.

- **Egress is an IP allowlist, default DNS only.** `pip install`, `npm install`, and `git clone` will time out unless the operator opened those registries. If a network call fails, say so and ask whether egress should be widened. Do not retry in a loop.
- **Standard library only.** No pandas, numpy, requests, or build tools in the image. Write stdlib Python (`csv`, `json`, `statistics`, `sqlite3`) and stdlib Node. If a task truly needs a third-party package, say the image would need rebuilding rather than trying to install it.
- **1 vCPU, 512 MB RAM, 2 GB disk.** Fine for scripts and CSVs up to a few hundred MB processed in a streaming way. Not enough for `next build`, large in-memory dataframes, or compiling big projects. Prefer streaming and chunked processing; if the job needs more, say so.
- **Files are capped at 8 MB each** via `sandbox_put_file`. For larger inputs, ask the user how to get the data in rather than splitting it silently.
- **Commands are request/response with a 60 s default timeout**, and the process is killed when the call returns. You cannot start a dev server, a daemon, or watch mode. For a web app, you can build and run tests, but not serve it. Pass `timeout_ms` for jobs over a minute.
- **Nothing can connect into a sandbox.** There is no URL for anything listening inside.
- **State under `/work` lasts about 15 minutes**, then the sandbox is destroyed. Copy results back into the conversation before you are done.

When a task cannot be done inside these limits, name the specific limit, propose the smallest change that would lift it, and stop. Do not fall back to running the code on the host unless the user explicitly asks.

## Reporting

Show the user the exact command and its `stdout`/`stderr`. Treat a non-zero `exit_code` as a failure to explain, not to hide. Output over a few KB should be summarized with the key lines quoted.
