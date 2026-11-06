import { useSyncExternalStore } from "react";

type Selection = Readonly<{
  project: string;
  session: string;
  assignee: string;
}>;
type SelectionStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const empty: Selection = { project: "", session: "", assignee: "" };

export class ResourceSelection {
  private value: Selection = empty;
  private listeners = new Set<() => void>();
  constructor(
    private key?: string,
    private storage?: SelectionStorage,
  ) {
    if (!key || !storage) return;
    try {
      const saved: unknown = JSON.parse(storage.getItem(key) ?? "null");
      if (
        saved &&
        typeof saved === "object" &&
        ["project", "session", "assignee"].every(
          (key) =>
            typeof (saved as Record<string, unknown>)[key] === "string" &&
            (saved as Record<string, string>)[key].length <= 256,
        )
      )
        this.value = {
          project: (saved as Selection).project,
          session: (saved as Selection).session,
          assignee: (saved as Selection).assignee,
        };
    } catch {
      /* Keep the in-memory selection when storage is unavailable. */
    }
  }
  snapshot = () => this.value;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private set(value: Selection) {
    if (
      value.project === this.value.project &&
      value.session === this.value.session &&
      value.assignee === this.value.assignee
    )
      return;
    this.value = value;
    try {
      if (this.key && this.storage) {
        if (value.project)
          this.storage.setItem(this.key, JSON.stringify(value));
        else this.storage.removeItem(this.key);
      }
    } catch {
      /* Selection remains usable with browser storage blocked. */
    }
    this.listeners.forEach((listener) => listener());
  }
  chooseProject(project: string) {
    if (project !== this.value.project) this.set({ ...empty, project });
  }
  chooseSession(project: string, session: string, assignee: string) {
    this.set({ project, session, assignee });
  }
  clear() {
    this.set(empty);
  }
}

export function createResourceSelection(baseUrl: string, identity?: string) {
  if (identity) {
    try {
      return new ResourceSelection(
        `cxz.resource-selection.v1:${encodeURIComponent(new URL(baseUrl).origin)}:${encodeURIComponent(identity)}`,
        sessionStorage,
      );
    } catch {
      /* Non-browser and isolated preview connections use memory only. */
    }
  }
  return new ResourceSelection();
}

export function useResourceSelection(c: { selection: ResourceSelection }) {
  return useSyncExternalStore(c.selection.subscribe, c.selection.snapshot);
}
