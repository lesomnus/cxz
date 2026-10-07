import {
  EMPTY_SETTINGS,
  parseSettings,
  type SettingsDocument,
} from "./editor-settings";

type Snapshot = {
  document: SettingsDocument;
  raw: string;
  valid: boolean;
  error: string;
};
type StorageFile = Pick<Storage, "getItem" | "setItem">;

export class SettingsStore {
  private value: Snapshot = {
    document: {},
    raw: EMPTY_SETTINGS,
    valid: true,
    error: "",
  };
  private listeners = new Set<() => void>();
  constructor(private storage: StorageFile) {
    try {
      this.sync(storage.getItem("settings"));
    } catch (error) {
      this.value = {
        ...this.value,
        valid: false,
        error: `설정을 읽을 수 없습니다: ${String(error)}`,
      };
    }
  }
  snapshot = () => this.value;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  sync(raw: string | null) {
    raw ??= EMPTY_SETTINGS;
    try {
      this.value = {
        document: parseSettings(raw),
        raw,
        valid: true,
        error: "",
      };
    } catch (error) {
      this.value = {
        document: {},
        raw,
        valid: false,
        error: `설정 파일 오류: ${String(error)}`,
      };
    }
    this.listeners.forEach((f) => f());
  }
  save(raw: string, expected?: string) {
    const document = parseSettings(raw);
    const current = this.storage.getItem("settings") ?? EMPTY_SETTINGS;
    if (expected !== undefined && expected !== current) {
      this.sync(current);
      throw new Error(
        "설정 파일이 변경되었습니다. 저장된 파일을 다시 읽은 뒤 수정하세요.",
      );
    }
    // Persist the entire file before notifying editors; failed writes never
    // claim success or apply an unsaved configuration.
    this.storage.setItem("settings", raw);
    this.value = { document, raw, valid: true, error: "" };
    this.listeners.forEach((f) => f());
  }
  set(key: string, value: unknown) {
    const raw = this.storage.getItem("settings") ?? EMPTY_SETTINGS;
    const document = parseSettings(raw);
    if (value === undefined) delete document[key];
    else document[key] = value;
    this.save(JSON.stringify(document, null, 2) + "\n", raw);
  }
}
