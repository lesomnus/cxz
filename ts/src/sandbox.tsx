import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { start, type Sandbox } from "@lesomnus/payday/sandbox";
import { Provider } from "@lesomnus/payday/react";
import { Button, Workspace } from "./app";
import { Connection } from "./connection";
import "./sandbox.css";
import workerURL from "./sandbox-worker.ts?worker&url";
import { boot, BootTimeout } from "./sandbox-boot";

const scenarios = [
  "Project checklist",
  "Conversation",
  "Long history",
  "Approval question",
  "Simulated error",
  "Seeded random work",
  "Stopped session",
  "Other project",
];
function SandboxApp() {
  const [seed, setSeed] = useState("42");
  const [delay, setDelay] = useState("400");
  const [scenario, setScenario] = useState("session-1");
  const [generation, setGeneration] = useState(0);
  const [connection, setConnection] = useState<Connection>();
  const [loading, setLoading] = useState("Starting sandbox…");
  // Kept rather than flashed: the first attempt stalling is the explanation for a
  // slow start, and it is worth still being on screen once the sandbox is up.
  const [stalled, setStalled] = useState(false);
  const [error, setError] = useState("");
  const [applied, setApplied] = useState({ seed: "42", delay: "400" });
  useEffect(() => {
    let canceled = false;
    let box: Sandbox | undefined;
    setConnection(undefined);
    setError("");
    setStalled(false);
    setLoading("Starting sandbox…");
    const worker = new URL(workerURL, location.href);
    worker.searchParams.set("seed", applied.seed);
    worker.searchParams.set("delay", applied.delay);
    // Bounded, and retried once without the module cache. Starting is the one
    // step with nothing above it to notice that it never finished: a stall in
    // the module's body or in the copy kept for the next reload leaves this page
    // on "Starting sandbox…" with no error and no progress.
    // The module cache makes a person's reload fast. Under automation it cannot:
    // every page is a fresh context, so each of the sandbox's browser tests
    // would write the 23 MB module into a cache that is thrown away -- while
    // sharing the module's stream with that write. Off there, kept here.
    const cacheable = !navigator.webdriver;
    boot(
      (cached) =>
        start({
          url: "/app.wasm",
          worker,
          wasmExec: "/wasm_exec.js",
          ...(cached && cacheable ? {} : { cache: false }),
          onProgress: (v) => {
            if (!canceled)
              setLoading(
                `Loading sandbox · ${(v.loaded / 1048576).toFixed(1)} MB${v.total ? ` / ${(v.total / 1048576).toFixed(1)} MB` : ""}`,
              );
          },
        }),
      {
        onRetry: () => {
          if (canceled) return;
          setStalled(true);
          setLoading("Starting sandbox again, without its cache…");
        },
      },
    )
      .then((v) => {
        if (canceled) {
          v.close();
          return;
        }
        box = v;
        setConnection(new Connection("https://sandbox.invalid", v.transport));
        setLoading("");
      })
      .catch((e) => {
        if (!canceled) {
          setError(
            e instanceof BootTimeout
              ? `${e.message}. Reset the sandbox to try again.`
              : String(e),
          );
          setLoading("");
        }
      });
    return () => {
      canceled = true;
      box?.close();
    };
  }, [generation, applied]);
  async function reset() {
    if (!/^\d{1,10}$/.test(seed) || Number(seed) > 4294967295) {
      setError("Seed must be an integer from 0 to 4294967295.");
      return;
    }
    setConnection(undefined);
    setApplied({ seed, delay });
    setGeneration((n) => n + 1);
  }
  return (
    <div className="sandbox-shell">
      <header className="sandbox-controls">
        <strong>Design sandbox</strong>
        <span>Fake agent · no accounts or real tasks</span>
        <label>
          Scenario
          <select
            aria-label="Scenario"
            value={scenario}
            onChange={(e) => setScenario(e.target.value)}
          >
            {scenarios.map((s, i) => (
              <option key={s} value={`session-${i + 1}`}>
                {s}
              </option>
            ))}
          </select>
        </label>
        <label>
          Seed
          <input
            aria-label="Seed"
            value={seed}
            inputMode="numeric"
            onChange={(e) => setSeed(e.target.value)}
          />
        </label>
        <label>
          Pace
          <select
            aria-label="Pace"
            value={delay}
            onChange={(e) => setDelay(e.target.value)}
          >
            <option value="100">Fast</option>
            <option value="400">Normal</option>
            <option value="1200">Slow</option>
          </select>
        </label>
        <Button onClick={() => void reset()}>Reset sandbox</Button>
      </header>
      {error && <p role="alert">{error}</p>}
      {stalled && !error && (
        <p role="note">
          The first attempt stalled; this sandbox started on a retry.
        </p>
      )}
      {loading && <p role="status">{loading}</p>}
      {connection && (
        <Provider key={connection.clientId} app={connection}>
          <Workspace
            key={scenario}
            connection={connection}
            initialSession={scenario}
            logout={reset}
            exitLabel="Reset sandbox"
          />
        </Provider>
      )}
    </div>
  );
}
createRoot(document.getElementById("root")!).render(<SandboxApp />);
