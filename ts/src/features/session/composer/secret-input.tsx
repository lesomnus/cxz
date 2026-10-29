import { useState } from "react";
import { Button } from "@lesomnus/nox";
import type { Connection } from "#src/shared/api/connection.ts";
import { secretFiles } from "./secret-paste";
import type { ComposerPaste } from "./composer-pastes";
import styles from "#src/shared/components/resource-surface.module.css";
export function SecretInput({
  c,
  project,
  session,
  run,
  insert,
  close,
}: {
  c: Connection;
  project: string;
  session: string;
  run: string;
  insert: (chip: ComposerPaste) => void;
  close: () => void;
}) {
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [progress, setProgress] = useState(0);
  return (
    <section className={styles.section}>
      <label className={styles.field}>
        Secret
        <input
          type="password"
          autoComplete="off"
          value={value}
          disabled={busy}
          onChange={(e) => setValue(e.target.value)}
        />
      </label>
      <small className={styles.muted}>
        Only the temporary file path enters the prompt. The agent can read this
        file and may expose its contents in its response.
      </small>
      {error && <p role="alert">{error}</p>}
      <Button
        disabled={busy || !value}
        onClick={async () => {
          const bytes = new TextEncoder().encode(value);
          if (bytes.length > 65536) {
            bytes.fill(0);
            setError("Secret limit: 64 KiB");
            return;
          }
          setBusy(true);
          setProgress(0);
          setError("");
          let path = "";
          try {
            path = await secretFiles(c, project, session, run).put(
              bytes,
              AbortSignal.timeout(30000),
              (loaded, total) => {
                setProgress(
                  total > 0
                    ? Math.min(99, Math.floor((loaded / total) * 100))
                    : 0,
                );
              },
            );
            if (!path) throw new Error("Secret path unavailable");
            insert({
              token: `[Secret ${crypto.randomUUID().replaceAll("-", "").slice(0, 8)}]`,
              body: `(secret placed at ${path})`,
              lines: 0,
              bytes: 0,
              secret: { session, run, path },
            });
            setValue("");
            close();
          } catch (error) {
            if (path)
              await secretFiles(c, project, session, run)
                .remove(path)
                .catch(() => {});
            setError(String(error));
          } finally {
            bytes.fill(0);
            setBusy(false);
          }
        }}
      >
        {busy
          ? `Uploading ${String(progress).padStart(3, "0")}%`
          : "Insert secret chip"}
      </Button>
    </section>
  );
}
