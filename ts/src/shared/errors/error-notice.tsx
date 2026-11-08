import { createElement, type HTMLAttributes, type ReactNode } from "react";
import { useLocalError } from "./error-context";

/** Local feedback; attended API failures explicitly opt in to inline display. */
export function ErrorNotice({
  error,
  children,
  as = "p",
  api = false,
  local = false,
  ...props
}: HTMLAttributes<HTMLElement> & {
  error: unknown;
  as?: "p" | "div" | "small" | "span";
  children?: ReactNode;
  api?: boolean;
  /** Show attended feedback here even when it is also captured centrally. */
  local?: boolean;
}) {
  const text = useLocalError(error, api, local);
  if (!text) return null;
  return createElement(as, { role: "alert", ...props }, children ?? text);
}
