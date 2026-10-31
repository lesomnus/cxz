import { useRef, useState } from "react";
import type { Session } from "#gen/cxz/session_pb";
import {
  canControl,
  controlTarget,
  performSessionControl,
  settleSessions,
  type SessionControlConnection,
  type SessionControlKind,
  type SessionTarget,
} from "#src/features/session/model/session-control-batch.ts";

export type ControlFailure = { target: SessionTarget; error: string };
export type ControlConfirmation = {
  kind: Exclude<SessionControlKind, "resume">;
  targets: SessionTarget[];
  failures: ControlFailure[];
};

export function useSessionControls(
  c: SessionControlConnection,
  succeeded: (ids: string[]) => void,
) {
  const lock = useRef(false);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [confirmation, setConfirmation] = useState<ControlConfirmation>();
  const [errors, setErrors] = useState<ControlFailure[]>([]);
  const acquire = (total: number) => {
    if (lock.current) return false;
    lock.current = true;
    setBusy(true);
    setErrors([]);
    setProgress({ done: 0, total });
    return true;
  };
  const release = () => {
    lock.current = false;
    setBusy(false);
  };
  const tick = () => setProgress((p) => ({ ...p, done: p.done + 1 }));

  async function execute(kind: SessionControlKind, targets: SessionTarget[]) {
    if (!acquire(targets.length)) return false;
    try {
      const results = await settleSessions(
        targets,
        (target) => performSessionControl(c, target, kind),
        tick,
      );
      succeeded(results.filter((r) => r.ok).map((r) => r.target.runtimeId));
      const failures = results
        .filter((r) => !r.ok)
        .map((r) => ({ target: r.target, error: r.error }));
      if (kind === "resume") setErrors(failures);
      else if (failures.length)
        setConfirmation(
          (previous) =>
            previous && {
              ...previous,
              targets: failures.map((f) => f.target),
              failures,
            },
        );
      return !failures.length;
    } finally {
      release();
    }
  }
  async function request(kind: SessionControlKind, sessions: Session[]) {
    if (lock.current) return;
    const targets = sessions
      .filter((s) => canControl(s, kind))
      .map(controlTarget);
    if (!targets.length) return;
    if (kind === "resume") {
      await execute(kind, targets);
      return;
    }
    setErrors([]);
    setConfirmation({ kind, targets, failures: [] });
  }
  return {
    busy,
    progress,
    confirmation,
    errors,
    request,
    execute,
    close: () => setConfirmation(undefined),
    dismissErrors: () => setErrors([]),
  };
}
