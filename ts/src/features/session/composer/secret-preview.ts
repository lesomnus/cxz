import type { ComposerPaste } from "./composer-pastes";
import type { SecretFiles } from "./secret-paste";

type Snapshot = {
  state: "loading" | "ready" | "error";
  value: string;
  error?: string;
};

// One preview owns its transient value. Closing the card aborts its read and
// drops the value; it never becomes part of ComposerPaste or SessionDrafts.
export class SecretPreview {
  private snapshot: Snapshot = { state: "loading", value: "" };
  private listeners = new Set<() => void>();
  constructor(
    private paste: ComposerPaste,
    private files?: SecretFiles,
  ) {}
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private update(snapshot: Snapshot) {
    this.snapshot = snapshot;
    this.listeners.forEach((listener) => listener());
  }
  connect = () => {
    const secret = this.paste.secret!;
    let active = true;
    let controller: AbortController | undefined;
    const load = () => {
      if (controller || !active) return;
      if (secret.state === "error") {
        this.update({
          state: "error",
          value: "",
          error: secret.error ?? "Secret preparation failed.",
        });
        return;
      }
      if (!secret.path || secret.state === "uploading") return;
      const files = this.files;
      if (
        !files?.read ||
        files.session !== secret.session ||
        files.run !== secret.run
      ) {
        this.update({
          state: "error",
          value: "",
          error: "Secret value unavailable for this session.",
        });
        return;
      }
      controller = new AbortController();
      this.update({ state: "loading", value: "" });
      void files
        .read(secret.path, controller.signal)
        .then((bytes) => {
          try {
            if (active)
              this.update({
                state: "ready",
                value: new TextDecoder("utf-8", {
                  fatal: true,
                  ignoreBOM: true,
                }).decode(bytes),
              });
          } finally {
            bytes.fill(0);
          }
        })
        .catch(() => {
          if (active)
            this.update({
              state: "error",
              value: "",
              error:
                "Secret value unavailable. The temporary file may have expired.",
            });
        });
    };
    const listeners = (secret.listeners ??= new Set());
    listeners.add(load);
    load();
    return () => {
      active = false;
      controller?.abort();
      listeners.delete(load);
      this.snapshot = { state: "loading", value: "" };
    };
  };
}
