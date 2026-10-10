import type { SettingsDocument } from "./editor-settings";

export function resolveTerminalSettings(document: SettingsDocument) {
  return { copyOnSelect: document["terminal.copyOnSelect"] !== false };
}
