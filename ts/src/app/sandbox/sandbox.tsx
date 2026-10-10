import { ThemeProvider } from "../../shared/theme/theme";
import { t, translateKnown } from "../../shared/i18n/i18n";
import { LocaleProvider, useLocale } from "../../shared/i18n/i18n-react";
import React, { useEffect, useState, type PropsWithChildren } from "react";
import { createRoot } from "react-dom/client";
import { start, type Sandbox } from "@lesomnus/payday/sandbox";
import { Provider } from "@lesomnus/payday/react";
import { Button } from "@lesomnus/cxz-ui";
import { Workspace } from "../workspace-shell";
import { WorkspaceView } from "../workspace-view";
import { Connection } from "../../shared/api/connection";
import "../styles/style.css";
import "./sandbox.css";
import workerURL from "./sandbox-worker.ts?worker&url";
import { boot, BootTimeout } from "./sandbox-boot";
import { createClient } from "@connectrpc/connect";
import { ProjectService } from "../../../gen/cxz/project_svc_pb";

import {
  createHashHistory,
  RouterProvider,
  useNavigate,
} from "@tanstack/react-router";
import { createWorkspaceRouter, useWorkspaceRoute } from "../router";

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
function SandboxApp({ children }: PropsWithChildren) {
  useLocale();
  const [seed, setSeed] = useState("42");
  const [delay, setDelay] = useState("400");
  const navigate = useNavigate();
  const route = useWorkspaceRoute();
  const [scenario, setScenario] = useState("session-1");
  useEffect(() => {
    if (route.session) setScenario(route.session);
  }, [route.session]);
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
    setLoading(t("Starting sandbox…"));
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
      async (cached, signal) => {
        const v = await start({
          url: "/app.wasm",
          worker,
          wasmExec: "/wasm_exec.js",
          ...(cached && cacheable ? {} : { cache: false }),
          onProgress: (v) => {
            if (!canceled && !signal.aborted)
              setLoading(
                t("Loading sandbox · {loaded} MB{total}", {
                  loaded: (v.loaded / 1048576).toFixed(1),
                  total: v.total
                    ? ` / ${(v.total / 1048576).toFixed(1)} MB`
                    : "",
                }),
              );
          },
        });
        const close = () => v.close();
        signal.addEventListener("abort", close, { once: true });
        try {
          signal.throwIfAborted();
          if (canceled) throw new Error("Sandbox start canceled");
          box = v;
          // Publishing a Go entry point does not prove the MessagePort can
          // answer requests. Keep the workspace behind the boot deadline until
          // a real RPC returns, so a stalled connection can be closed/retried.
          await createClient(ProjectService, v.transport).list(
            { size: 1 },
            { signal },
          );
          if (canceled) throw new Error("Sandbox start canceled");
          return v;
        } catch (e) {
          v.close();
          throw e;
        } finally {
          signal.removeEventListener("abort", close);
        }
      },
      {
        onRetry: () => {
          if (canceled) return;
          setStalled(true);
          setLoading(t("Starting sandbox again, without its cache…"));
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
            onChange={(e) => {
              setScenario(e.target.value);
              void navigate({
                to: "/sessions/$sessionId",
                params: { sessionId: e.target.value },
              });
            }}
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
      {stalled && !error && (
        <p role="note">
          {t("The first attempt stalled; this sandbox started on a retry.")}
        </p>
      )}
      {loading && <p role="status">{translateKnown(loading)}</p>}

      {connection && (
        <Provider key={connection.clientId} app={connection}>
          <Workspace
            connection={connection}
            logout={reset}
            exitLabel={t("Reset sandbox")}
          >
            {children}
          </Workspace>
        </Provider>
      )}
    </div>
  );
}
const history = createHashHistory();
if (!location.hash) history.replace("/sessions/session-1");
const router = createWorkspaceRouter(
  { shell: SandboxApp, view: WorkspaceView },
  history,
);
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <LocaleProvider>
      <RouterProvider router={router} />
    </LocaleProvider>
  </ThemeProvider>,
);
