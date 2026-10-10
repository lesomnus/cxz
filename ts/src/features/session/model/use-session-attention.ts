import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { Connection } from "#src/shared/api/connection.ts";
import type { Session } from "#gen/cxz/session_pb";
import { sessionSound } from "#src/shared/notifications/session-sound.ts";
import { tabAttention } from "#src/shared/notifications/tab-attention.ts";
import { useSettings } from "#src/shared/settings/settings.ts";
import { questions } from "./journal";

export function useSessionAttention(
  c: Connection,
  sessions: Session[],
  active: string,
) {
  const unread = useSyncExternalStore(
    c.attention.subscribe,
    c.attention.snapshot,
  );
  const { snapshot } = useSettings();
  const enabled = useRef(true);
  enabled.current = snapshot.document["notifications.sound"] !== false;
  const [focused, setFocused] = useState(
    () => document.visibilityState === "visible" && document.hasFocus(),
  );
  const badge = useRef<ReturnType<typeof tabAttention>>(undefined);
  useEffect(() => {
    const update = () =>
      setFocused(document.visibilityState === "visible" && document.hasFocus());
    window.addEventListener("focus", update);
    window.addEventListener("blur", update);
    document.addEventListener("visibilitychange", update);
    const sound = sessionSound(() => enabled.current);
    const stop = c.attention.onAlert((kind) => sound.play(kind));
    badge.current = tabAttention();
    return () => {
      window.removeEventListener("focus", update);
      window.removeEventListener("blur", update);
      document.removeEventListener("visibilitychange", update);
      stop();
      sound.dispose();
      badge.current?.dispose();
    };
  }, [c]);
  useEffect(() => {
    c.attention.observe(sessions);
  }, [c, sessions]);
  useEffect(() => {
    const ids = new Set(unread);
    for (const s of sessions) {
      if (
        (!focused || s.runtimeId !== active) &&
        s.status?.pending.some((e) => questions(s.agent, e).length)
      )
        ids.add(s.runtimeId);
    }
    badge.current?.update(ids.size);
  }, [unread, sessions, active, focused]);
  return unread;
}
