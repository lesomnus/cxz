import { createContext, useContext, useMemo, type ReactNode } from "react";

export type UIMessage =
  | "Cancel"
  | "Confirm"
  | "Working…"
  | "Action failed. Please try again."
  | "Edit {label}"
  | "{label} choices"
  | "Inherited: {value}"
  | "Reset to inherited value";
export type UITranslate = (
  message: UIMessage,
  values?: Record<string, string | number>,
) => string;
const english: UITranslate = (message, values = {}) =>
  message.replace(/\{(\w+)\}/g, (match, key) => String(values[key] ?? match));
const UIContext = createContext({ t: english });

// Optional: components work with English defaults without a provider or app store.
export function UIProvider({
  translate = english,
  locale,
  children,
}: {
  translate?: UITranslate;
  locale?: string;
  children: ReactNode;
}) {
  const value = useMemo(() => ({ t: translate }), [translate, locale]);
  return <UIContext.Provider value={value}>{children}</UIContext.Provider>;
}
export function useUI() {
  return useContext(UIContext);
}
