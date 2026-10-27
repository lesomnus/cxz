import { useEffect, useRef, useState } from "react";
import { Button, Section } from "@lesomnus/nox";
import type { Connection } from "#src/shared/api/connection.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import styles from "#src/shared/components/resource-surface.module.css";

export type LoginTarget = { account: string; project?: string; key?: string };
export function LoginPanel({
  c,
  target,
  complete,
  cancel,
}: {
  c: Connection;
  target: LoginTarget;
  complete: () => void;
  cancel: () => void;
}) {
  const [attempt, setAttempt] = useState(0);
  const [output, setOutput] = useState("");
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  const [code, setCode] = useState("");
  const socket = useRef<WebSocket | undefined>(undefined);
  const onComplete = useRef(complete);
  onComplete.current = complete;
  useEffect(() => {
    if (c.inProcess) {
      setError("Provider login requires a real Manager.");
      return;
    }
    setOutput("");
    setError("");
    setCode("");
    setReady(false);
    const url = new URL(
      `/login/${target.project ? "session" : "auxiliary"}`,
      c.baseUrl,
    );
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    url.searchParams.set("account", target.account);
    if (target.project) {
      url.searchParams.set("project", target.project);
      url.searchParams.set("key", target.key!);
    }
    const ws = new WebSocket(url);
    socket.current = ws;
    let ended = false;
    ws.onopen = () => setReady(true);
    ws.onmessage = (event) => {
      if (ended) return;
      let frame;
      try {
        frame = JSON.parse(String(event.data));
      } catch {
        ended = true;
        setReady(false);
        setError("Invalid login response.");
        ws.close();
        return;
      }
      if (frame.output)
        setOutput((previous) => (previous + frame.output).slice(-65536));
      if (frame.error) {
        ended = true;
        setReady(false);
        setError(frame.error);
        ws.close();
        return;
      }
      if (frame.done) {
        ended = true;
        setReady(false);
        ws.close();
        onComplete.current();
      }
    };
    ws.onerror = () => {
      ended = true;
      setReady(false);
      setError("Login connection failed.");
    };
    ws.onclose = () => {
      setReady(false);
      if (!ended) setError("Login connection closed before completion.");
    };
    return () => {
      ended = true;
      ws.onmessage = null;
      ws.onerror = null;
      ws.onclose = null;
      ws.close();
    };
  }, [c, target.account, target.project, target.key, attempt]);
  return (
    <Section aria-label="Provider login">
      <strong>Log in · {target.account}</strong>
      <div className={styles.status}>
        <Markdown text={output.replace(/\x1b\[[0-9;]*[A-Za-z]/g, "")} />
      </div>
      {error && <p role="alert">{error}</p>}
      <label className={styles.field}>
        Authentication code
        <input
          type="password"
          autoComplete="off"
          value={code}
          onChange={(e) => setCode(e.target.value)}
        />
      </label>
      <div className={styles.row}>
        <Button
          type="button"
          disabled={!ready || !code}
          onClick={() => {
            socket.current?.send(code + "\n");
            setCode("");
          }}
        >
          Submit code
        </Button>
        {error && !c.inProcess && (
          <Button
            type="button"
            onClick={() => setAttempt((value) => value + 1)}
          >
            Retry login
          </Button>
        )}
        <Button type="button" onClick={cancel}>
          Cancel
        </Button>
      </div>
    </Section>
  );
}
