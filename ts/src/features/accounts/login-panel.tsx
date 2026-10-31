import { useContext, useEffect, useRef, useState } from "react";
import {
  Button,
  Dialog,
  DialogBody,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Section,
} from "@lesomnus/nox";
import type { Connection } from "#src/shared/api/connection.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
import { LoginTransportContext, type LoginSocket } from "./login-transport";

export type LoginTarget = { account: string; project?: string; key?: string };
export function LoginPanel({
  c,
  target,
  complete,
  cancel,
  modal = false,
}: {
  c: Connection;
  target: LoginTarget;
  complete: () => void;
  cancel: () => void;
  modal?: boolean;
}) {
  const [attempt, setAttempt] = useState(0);
  const [output, setOutput] = useState("");
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  const [code, setCode] = useState("");
  const connect = useContext(LoginTransportContext);
  const socket = useRef<LoginSocket | undefined>(undefined);
  const input = useRef<HTMLInputElement>(null);
  const onComplete = useRef(complete);
  onComplete.current = complete;
  useEffect(() => {
    if (c.inProcess && !connect) {
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
    const ws = connect ? connect(url) : new WebSocket(url);
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
      if (socket.current === ws) socket.current = undefined;
    };
  }, [c, target.account, target.project, target.key, attempt, connect]);
  const submitCode = () => {
    if (!ready || !code) return;
    socket.current?.send(code + "\n");
    setCode("");
  };
  const fields = (
    <>
      <div className={styles.status}>
        <Markdown text={output.replace(/\x1b\[[0-9;]*[A-Za-z]/g, "")} />
      </div>
      {error && <p role="alert">{error}</p>}
      <label className={styles.field}>
        Authentication code
        <input
          ref={input}
          type="password"
          autoComplete="off"
          value={code}
          onChange={(e) => setCode(e.target.value)}
        />
      </label>
    </>
  );
  const actions = (
    <>
      <Button
        type={modal ? "submit" : "button"}
        disabled={!ready || !code}
        onClick={modal ? undefined : submitCode}
      >
        Submit code
      </Button>
      {error && (!c.inProcess || connect) && (
        <Button type="button" onClick={() => setAttempt((value) => value + 1)}>
          Retry login
        </Button>
      )}
      <Button type="button" onClick={cancel}>
        Cancel
      </Button>
    </>
  );
  if (modal)
    return (
      <Dialog open onClose={cancel} initialFocus={input}>
        <form
          role="region"
          aria-label="Provider login"
          onSubmit={(event) => {
            event.preventDefault();
            submitCode();
          }}
        >
          <DialogHeader>
            <DialogTitle>Log in · {target.account}</DialogTitle>
          </DialogHeader>
          <DialogBody className={styles.list}>{fields}</DialogBody>
          <DialogFooter>{actions}</DialogFooter>
        </form>
      </Dialog>
    );
  return (
    <Section aria-label="Provider login">
      <strong>Log in · {target.account}</strong>
      {fields}
      <div className={styles.row}>{actions}</div>
    </Section>
  );
}
