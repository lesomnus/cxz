import { pasteRanges, type ComposerPaste } from "../composer/composer-pastes";
import {
  validEditorSelection,
  type EditorSelection,
} from "#src/features/session/composer/editor-selection.ts";

type SavedPaste = Omit<ComposerPaste, "attachment"> & {
  attachment?: Pick<
    NonNullable<ComposerPaste["attachment"]>,
    "name" | "directory" | "path" | "state"
  >;
};

// Tab-local storage survives refresh without making another tab overwrite an
// editor, or persisting credentials and conversation history alongside drafts.
export class SessionDrafts extends Map<string, string> {
  private prefix: string;
  private selections = new Map<string, EditorSelection>();
  constructor(
    scope: string,
    private pastes: Map<string, ComposerPaste>,
    private storage: () => Storage = () => sessionStorage,
  ) {
    super();
    this.prefix = `cxz.draft.v1:${encodeURIComponent(scope)}:`;
  }
  override get(id: string): string | undefined {
    if (super.has(id)) return super.get(id);
    try {
      const raw = this.storage().getItem(this.prefix + encodeURIComponent(id));
      if (!raw) return undefined;
      const saved = JSON.parse(raw);
      if (typeof saved.text !== "string" || !Array.isArray(saved.pastes))
        return undefined;
      for (const savedPaste of saved.pastes as SavedPaste[]) {
        const p = {
          ...savedPaste,
          sendAs:
            savedPaste.sendAs === "text" || savedPaste.sendAs === "file"
              ? savedPaste.sendAs
              : undefined,
        };
        if (
          typeof p.token !== "string" ||
          typeof p.body !== "string" ||
          !Number.isFinite(p.bytes) ||
          !Number.isFinite(p.lines)
        )
          continue;
        const attachment = p.attachment;
        if (attachment) {
          if (typeof attachment.name !== "string") continue;
          const ready =
            attachment.state === "ready" &&
            typeof attachment.path === "string" &&
            attachment.path.startsWith("/");
          this.pastes.set(p.token, {
            ...p,
            attachment: {
              ...attachment,
              file: p.body ? new File([p.body], attachment.name) : undefined,
              state: ready ? "ready" : "error",
              error: ready
                ? undefined
                : "Upload interrupted. Remove this chip and select the file again.",
            },
          });
        } else this.pastes.set(p.token, p);
      }
      super.set(id, saved.text);
      return saved.text;
    } catch {
      return undefined;
    }
  }
  override set(id: string, text: string) {
    super.set(id, text);
    if (!text) this.clearSelection(id);
    this.persist(id);
    return this;
  }
  selection(id: string): EditorSelection | undefined {
    if (this.selections.has(id)) return this.selections.get(id);
    try {
      const raw = this.storage().getItem(
        this.prefix + "selection:" + encodeURIComponent(id),
      );
      const value: unknown = raw ? JSON.parse(raw) : undefined;
      if (validEditorSelection(value)) {
        this.selections.set(id, value);
        return value;
      }
    } catch {
      /* Keep the editor usable with unavailable storage. */
    }
  }
  setSelection(id: string, selection: EditorSelection) {
    if (
      !(super.get(id) ?? "") &&
      selection.start === 0 &&
      selection.end === 0
    ) {
      this.clearSelection(id);
      return;
    }
    this.selections.set(id, selection);
    // Cursor movement writes only a small record, not the text or pasted files.
    try {
      this.storage().setItem(
        this.prefix + "selection:" + encodeURIComponent(id),
        JSON.stringify(selection),
      );
    } catch {
      /* memory still works */
    }
  }
  private clearSelection(id: string) {
    this.selections.delete(id);
    try {
      this.storage().removeItem(
        this.prefix + "selection:" + encodeURIComponent(id),
      );
    } catch {
      /* unavailable */
    }
  }
  persist(id: string) {
    try {
      const text = super.get(id) ?? "";
      const key = this.prefix + encodeURIComponent(id);
      if (!text) {
        this.storage().removeItem(key);
        return;
      }
      const pastes: SavedPaste[] = pasteRanges(text, this.pastes).map(
        ({ paste: p }) => ({
          token: p.token,
          body: p.body,
          lines: p.lines,
          bytes: p.bytes,
          sendAs: p.sendAs,
          secret: p.secret,
          attachment: p.attachment && {
            name: p.attachment.name,
            directory: p.attachment.directory,
            path: p.attachment.path,
            state: p.attachment.state,
          },
        }),
      );
      this.storage().setItem(key, JSON.stringify({ text, pastes }));
    } catch {
      /* Restricted/full storage still leaves the in-memory draft usable. */
    }
  }
  override delete(id: string) {
    this.clearSelection(id);
    try {
      this.storage().removeItem(this.prefix + encodeURIComponent(id));
    } catch {
      /* unavailable */
    }
    return super.delete(id);
  }
  override clear() {
    try {
      const storage = this.storage();
      for (let i = storage.length - 1; i >= 0; i--) {
        const key = storage.key(i);
        if (key?.startsWith(this.prefix)) storage.removeItem(key);
      }
    } catch {
      /* unavailable */
    }
    super.clear();
    this.selections.clear();
  }
}
