import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useContext, useEffect, useId, useRef, useState } from "react";
import {
  Button,
  Dialog,
  DialogBody,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormField,
  ScrollArea,
  Section,
} from "@lesomnus/nox";
import type { Connection } from "#src/shared/api/connection.ts";
import { useDialogClose } from "#src/shared/components/dialog-close.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
import loginStyles from "./login-panel.module.css";
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
  const dialogClose = useDialogClose(cancel);
  const [attempt, setAttempt] = useState(0);
  const [output, setOutput] = useState("");
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  const [code, setCode] = useState("");
  const connect = useContext(LoginTransportContext);
  const socket = useRef<LoginSocket | undefined>(undefined);
  const stopLogin = useRef<(() => void) | undefined>(undefined);
  const input = useRef<HTMLInputElement>(null);
  const codeId = useId();
  const onComplete = useRef(complete);
  const failureKey = JSON.stringify([
    c.baseUrl,
    "provider-login",
    target.account,
    target.project ?? "",
  ]);
  onComplete.current = modal ? () => dialogClose.complete(complete) : complete;
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
    const failed = (message: string) => {
      c.errors.report(failureKey, new Error(message), {
        source: "http",
        connection: c.baseUrl,
        method: "Provider login",
        project: target.project,
        target: `Account: ${target.account}`,
      });
      setError(message);
    };
    let ws: LoginSocket;
    try {
      ws = connect ? connect(url) : new WebSocket(url);
    } catch (e) {
      failed(String(e));
      return;
    }
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
        failed("Invalid login response.");
        ws.close();
        return;
      }
      if (frame.output)
        setOutput((previous) => (previous + frame.output).slice(-65536));
      if (frame.error) {
        ended = true;
        setReady(false);
        failed(frame.error);
        ws.close();
        return;
      }
      if (frame.done) {
        c.errors.resolve(failureKey);
        ended = true;
        setReady(false);
        ws.close();
        onComplete.current();
      }
    };
    ws.onerror = () => {
      ended = true;
      setReady(false);
      failed("Login connection failed.");
    };
    ws.onclose = () => {
      setReady(false);
      if (!ended) failed("Login connection closed before completion.");
    };
    const stop = () => {
      ended = true;
      ws.onmessage = null;
      ws.onerror = null;
      ws.onclose = null;
      ws.close();
      if (socket.current === ws) socket.current = undefined;
    };
    stopLogin.current = stop;
    return () => {
      stop();
      if (stopLogin.current === stop) stopLogin.current = undefined;
    };
  }, [c, target.account, target.project, target.key, attempt, connect]);
  const dismissLogin = () => {
    // Cancel authentication immediately; only visual cleanup waits for the exit.
    stopLogin.current?.();
    if (modal) dialogClose.dismiss();
    else cancel();
  };
  const submitCode = () => {
    if (!ready || !code) return;
    socket.current?.send(code + "\n");
    setCode("");
  };
  const instructions = (
    <>
      <div className={styles.status}>
        <Markdown text={output.replace(/\x1b\[[0-9;]*[A-Za-z]/g, "")} />
      </div>
      {error && <ErrorNotice error={error} />}
    </>
  );
  const codeField = (
    <Form as="fieldset" columns={1} className={loginStyles.code}>
      <FormField label="Authentication code" htmlFor={codeId}>
        <input
          id={codeId}
          ref={input}
          type="password"
          autoComplete="off"
          value={code}
          onChange={(e) => setCode(e.target.value)}
        />
      </FormField>
    </Form>
  );
  const canRetry = !!error && (!c.inProcess || !!connect);
  const retry = (modal || canRetry) && (
    <Button
      type="button"
      className={modal && !canRetry ? loginStyles.reservedAction : undefined}
      onClick={() => setAttempt((value) => value + 1)}
    >
      Retry login
    </Button>
  );
  const actions = (
    <>
      {modal && retry}
      <Button
        type={modal ? "submit" : "button"}
        disabled={!ready || !code}
        onClick={modal ? undefined : submitCode}
      >
        Submit code
      </Button>
      {!modal && retry}
      <Button type="button" onClick={dismissLogin}>
        Cancel
      </Button>
    </>
  );
  if (modal)
    return (
      <Dialog
        open={dialogClose.open}
        onClose={dismissLogin}
        onClosed={dialogClose.onClosed}
        initialFocus={input}
      >
        <form
          className={loginStyles.form}
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
          <DialogBody className={loginStyles.body}>
            <ScrollArea
              className={loginStyles.scroll}
              contentClassName={loginStyles.instructions}
              scrollbarLabel="Login instructions"
            >
              {instructions}
            </ScrollArea>
            {codeField}
          </DialogBody>
          <DialogFooter>{actions}</DialogFooter>
        </form>
      </Dialog>
    );
  return (
    <Section aria-label="Provider login">
      <strong>Log in · {target.account}</strong>
      {instructions}
      {codeField}
      <div className={styles.row}>{actions}</div>
    </Section>
  );
}
