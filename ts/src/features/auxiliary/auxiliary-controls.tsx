import { useEffect, useState } from "react";
import { Button } from "@lesomnus/nox";
import { AuxKind } from "#gen/cxz/project_svc_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useSessionAux } from "#src/features/session/model/use-session-aux.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
export const auxKinds = [
  { kind: AuxKind.SUMMARY, label: "Summary" },
  { kind: AuxKind.SUGGESTION, label: "Suggestion" },
  { kind: AuxKind.TITLE, label: "Title" },
];
export function AuxiliaryControls({
  c,
  session,
  insert,
}: {
  c: Connection;
  session: string;
  insert?: (text: string) => void;
}) {
  const stream = useSessionAux(c, session);
  const [changed, setChanged] = useState<typeof stream>();
  const state = changed ?? stream;
  useEffect(() => setChanged(undefined), [stream]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function act(run: () => Promise<NonNullable<typeof stream>>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      setChanged(await run());
    } catch (error) {
      setError(String(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className={styles.section} aria-label="Auxiliary AI">
      <strong>Auxiliary AI</strong>
      {auxKinds.map(({ kind, label }) => (
        <div key={kind} className={styles.row}>
          <span>{label}</span>
          <label>
            <input
              type="checkbox"
              checked={
                state?.preferences.find((p) => p.kind === kind)?.enabled ??
                false
              }
              disabled={busy || !state}
              onChange={(e) =>
                void act(() =>
                  c.sessions.auxPrefer(
                    {
                      ref: ref(session),
                      preferences: [{ kind, enabled: e.target.checked }],
                    },
                    { timeoutMs: 30000 },
                  ),
                )
              }
            />{" "}
            Automatic
          </label>
          <Button
            disabled={busy}
            onClick={() =>
              void act(() =>
                c.sessions.auxRun(
                  { ref: ref(session), kinds: [kind] },
                  { timeoutMs: 20000 },
                ),
              )
            }
          >
            Run now
          </Button>
        </div>
      ))}
      {error && <p role="alert">{error}</p>}
      {state?.message && <p className={styles.muted}>{state.message}</p>}
      {state?.current && (
        <div>
          <div className={styles.row}>
            <span className={styles.status}>{state.current.state}</span>
            {["queued", "running"].includes(state.current.state) && (
              <Button
                disabled={busy}
                onClick={() =>
                  void act(() =>
                    c.sessions.auxCancel(
                      {
                        ref: ref(session),
                        auxId: state.current!.id,
                      },
                      { timeoutMs: 30000 },
                    ),
                  )
                }
              >
                Cancel task
              </Button>
            )}
          </div>
          {state.current.message && <p>{state.current.message}</p>}
        </div>
      )}
      {(state?.recent ?? []).map((task) => (
        <div key={task.id}>
          <small className={styles.status}>
            {task.state} · turn {task.turn.toString()}
          </small>
          {task.results.map((result) => (
            <div key={result.kind}>
              <strong>
                {auxKinds.find((k) => k.kind === result.kind)?.label}
              </strong>
              <Markdown text={result.text} />
              {result.kind === AuxKind.SUGGESTION && insert && (
                <Button onClick={() => insert(result.text)}>
                  Insert suggestion
                </Button>
              )}
              {result.truncated && (
                <small className={styles.muted}>Result was truncated.</small>
              )}
            </div>
          ))}
          {task.usage.length > 0 && (
            <details>
              <summary>Task usage</summary>
              {task.usage.map((usage, index) => (
                <pre key={index} className={styles.status}>
                  {usage.model} · {usage.account}
                  {"\n"}
                  {new TextDecoder().decode(usage.data)}
                </pre>
              ))}
            </details>
          )}
        </div>
      ))}
    </section>
  );
}
