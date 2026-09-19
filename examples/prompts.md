# Ten prompts to try from Claude Code

Open Claude Code anywhere on a machine with the `sandbox` MCP server registered and the tunnel up
(`ssh -N sandboxd`). Paste one prompt at a time. Each says what should happen.

Prompts 1 to 7 fit inside the current limits and should just work. Prompts 8 to 10 deliberately hit
a limit; the skill should make Claude name the limit and stop rather than retry or fall back to
host Bash.

---

**1. Smoke test**

> Run `uname -a`, `id`, `python3 --version`, and `node --version` in the sandbox and show me the output.

Expect: Linux 6.1.141 kernel, uid 0, Python 3.11, Node 18. No files created.

**2. Write and run a script**

> Write a Python script that generates 1000 random 3D points, finds the pair with the smallest distance using only the standard library, and prints the pair and distance. Run it in the sandbox.

Expect: `sandbox_put_file` then `sandbox_run`, real stdout, exit 0. Naive O(n²) is fine at n=1000.

**3. CSV crunching, stdlib only**

> Create a CSV with 50,000 rows of fake sales data (date, region, product, units, unit_price) in the sandbox, then compute revenue per region per month and print the top 10 rows sorted by revenue. No pandas.

Expect: two scripts or one, `csv` and `collections` modules, a few seconds runtime, a printed table.

**4. SQLite pipeline**

> In the sandbox, load that sales CSV into SQLite, add an index, and answer: which product had the highest average unit price in each region? Show the SQL and the result.

Expect: `sqlite3` module, `/work/sales.csv` still present from prompt 3 if under 15 minutes, otherwise Claude regenerates it.

**5. Node without npm**

> Write a Node script that parses this JSON and prints a Markdown table of the three most common values of `status`, then run it in the sandbox: `[{"status":"ok"},{"status":"fail"},{"status":"ok"},{"status":"skip"},{"status":"ok"},{"status":"fail"}]`

Expect: stdlib Node, no `npm install`, exit 0.

**6. Shell pipeline and file persistence**

> In the sandbox, create 200 files named `log-NNN.txt`, each with a random number of lines containing either INFO or ERROR. Then use only shell tools to report how many files have more than 5 ERROR lines. Then list `/work`.

Expect: `sh -c` with a loop, `grep -c`, `awk`, then `ls`. Shows the same sandbox is reused across calls.

**7. Long-running job with a timeout**

> Run a Python loop in the sandbox that sums the squares of the first 300 million integers. It will take more than a minute, so set an appropriate timeout.

Expect: Claude passes `timeout_ms` above 60000. On 1 vCPU nested KVM this takes roughly 60 to 120 seconds. If Claude forgets the timeout, the result comes back with `error: "timeout"` and it should retry once with a bigger value.

---

**8. Package install (hits the egress limit)**

> Install pandas in the sandbox and use it to compute the mean of a column in a CSV.

Expect: `pip install pandas` times out. Claude should say egress is an IP allowlist with PyPI closed, offer either a stdlib `csv` version or asking the operator to widen egress, and not retry in a loop.

**9. Dev server (hits the process model limit)**

> Create a minimal Express-free Node HTTP server on port 3000 in the sandbox, start it, and give me the URL.

Expect: Claude can write it and even run it briefly, but should explain that commands are request/response with the process killed on return, and that nothing can connect into a sandbox, so there is no URL. It should not try `nohup` tricks or run the server on the host.

**10. Big file (hits the upload cap)**

> Upload this 50 MB CSV I have locally at ~/data/big.csv into the sandbox and count its rows.

Expect: Claude should stop before uploading and say files are capped at 8 MB per `sandbox_put_file`, then ask how you want to get the data in. It should not split the file silently or read it with host Bash and pipe the count back as if it ran in the sandbox.

---

## Watching from the box side

While you run these, in another terminal:

```sh
ssh sandboxd
sudo journalctl -u sandboxd -f -o cat | grep -a "ready in\|pool fill"   # if installed via install.sh
tail -f /tmp/sandboxd.log | grep -a "ready in"                            # if started by hand
```

Every prompt that touches a fresh sandbox should show a `ready in ~100ms` line as the pool refills.
