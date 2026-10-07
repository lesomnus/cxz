import { t, translateKnown } from "./i18n";
import { LocaleProvider, useLocale } from "./i18n-react";
import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { start, type Sandbox } from "@lesomnus/payday/sandbox";
import { Provider } from "@lesomnus/payday/react";
import { Button, Workspace } from "./app";
import { Connection } from "./connection";
import "./sandbox.css";
import workerURL from "./sandbox-worker.ts?worker&url";

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
  useLocale();
  const [seed, setSeed] = useState("42");
  const [delay, setDelay] = useState("400");
  const [scenario, setScenario] = useState("session-1");
  const [generation, setGeneration] = useState(0);
  const [connection, setConnection] = useState<Connection>();
  const [loading, setLoading] = useState("Starting sandbox…");
  const [error, setError] = useState("");
  const [applied, setApplied] = useState({ seed: "42", delay: "400" });
  useEffect(() => {
    let canceled = false;
    let box: Sandbox | undefined;
    setConnection(undefined);
    setError("");
    setLoading(t("Starting sandbox…"));
    const worker = new URL(workerURL, location.href);
    worker.searchParams.set("seed", applied.seed);
    worker.searchParams.set("delay", applied.delay);
    start({
      url: "/app.wasm",
      worker,
      wasmExec: "/wasm_exec.js",
      onProgress: (v) => {
        if (!canceled)
          setLoading(
            t("Loading sandbox · {loaded} MB{total}", {
              loaded: (v.loaded / 1048576).toFixed(1),
              total: v.total ? ` / ${(v.total / 1048576).toFixed(1)} MB` : "",
            }),
          );
      },
    })
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
          setError(String(e));
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
      setError(t("Seed must be an integer from 0 to 4294967295."));
      return;
    }
    setConnection(undefined);
    setApplied({ seed, delay });
    setGeneration((n) => n + 1);
  }
  return (
    <div className="sandbox-shell">
      <header className="sandbox-controls">
        <strong>{t("Design sandbox")}</strong>
        <span>{t("Fake agent · no accounts or real tasks")}</span>
        <label>
          {t("Scenario")}
          <select
            aria-label={t("Scenario")}
            value={scenario}
            onChange={(e) => setScenario(e.target.value)}
          >
            {scenarios.map((s, i) => (
              <option key={s} value={`session-${i + 1}`}>
                {translateKnown(s)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("Seed")}
          <input
            aria-label={t("Seed")}
            value={seed}
            inputMode="numeric"
            onChange={(e) => setSeed(e.target.value)}
          />
        </label>
        <label>
          {t("Pace")}
          <select
            aria-label={t("Pace")}
            value={delay}
            onChange={(e) => setDelay(e.target.value)}
          >
            <option value="100">{t("Fast")}</option>
            <option value="400">{t("Normal")}</option>
            <option value="1200">{t("Slow")}</option>
          </select>
        </label>
        <Button onClick={() => void reset()}>{t("Reset sandbox")}</Button>
      </header>
      {error && <p role="alert">{error}</p>}
      {loading && <p role="status">{translateKnown(loading)}</p>}
      {connection && (
        <Provider key={connection.clientId} app={connection}>
          <Workspace
            key={scenario}
            connection={connection}
            initialSession={scenario}
            logout={reset}
            exitLabel={t("Reset sandbox")}
          />
        </Provider>
      )}
    </div>
  );
}
createRoot(document.getElementById("root")!).render(
  <LocaleProvider>
    <SandboxApp />
  </LocaleProvider>,
);
