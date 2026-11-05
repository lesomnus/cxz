import { createElement, type HTMLAttributes, type ReactNode } from "react";
import { useLocalError } from "./error-context";

/** Show local input/browser feedback without duplicating a captured API failure. */
export function ErrorNotice({
  error,
  children,
  as = "p",
  api = false,
  ...props
}: HTMLAttributes<HTMLElement> & {
  error: unknown;
  as?: "p" | "div" | "small" | "span";
  children?: ReactNode;
  api?: boolean;
}) {
  const text = useLocalError(error, api);
  if (!text) return null;
  return createElement(as, { role: "alert", ...props }, children ?? text);
}
