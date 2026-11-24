# Session creation and execution

`SessionService.Add` stores a session resource. It validates the project, account,
authentication binding and requested settings, assigns an alias, and returns the
session in `created` state. It does not connect to the project runtime, install
an agent, check provider login, or launch a supervisor. The project can be
stopped or have no container when the resource is added.

`SessionService.Start` starts that stored session. On its first start it prepares
the agent and authentication in an already running project, writes the runtime
manifest and launches the supervisor. It returns after the runtime leaves its
initial starting state. Starting a stopped session uses the existing runtime
manifest. `Resume` remains available and delegates to the same implementation.
Neither RPC starts the project container; use `ProjectService.Start` for that.

The creation `client_id` identifies the durable session and makes Add retries
converge on that record. A different creation request reusing the key is rejected.
The manager and project runtime reserve the same runtime identity from that key,
so a failed first start can be retried without creating another session. Existing
sessions keep their original identities.

An unstarted session can be listed, renamed or erased without a runtime. A failed
Start leaves the resource available for retry. A login challenge is returned by
Start; after login, retry Start on the existing session. Canceling login leaves
the resource in place. Before the first run, history is empty and live streams
wait with heartbeats until the runtime is available.

The web creation form calls AuthBinding.Add and Session.Add, then closes and
opens the stored session's conversation. The conversation calls Session.Start
once on entry and presents any session login challenge there. Login completion
retries Start on that session.
The CLI/TUI creation adapters also compose Add and Start explicitly. Storybook
provides separate creation and start delays, and the WASM sandbox implements the
same split without running an agent.

## When a project runtime is slow

The Manager refreshes every project's session states at once, giving each
project runtime five seconds to answer. A runtime that is reachable but misses
the deadline -- usually because the host is short of CPU or memory -- keeps its
sessions' last known states and logs `runtime did not list its sessions`; a
loaded host is not a reason to report its sessions as stopped, which would hide
them from views that leave out stopped sessions. A runtime still silent five
minutes later, or one whose container is not running or refuses connections,
has its running sessions reported as `interrupted`.
