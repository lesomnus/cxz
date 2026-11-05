import {
  createContext,
  useContext,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { isAPIError, type ClientErrors } from "./client-errors";

const ErrorContext = createContext<ClientErrors | null>(null);
export const useErrorCenter = () => useContext(ErrorContext);
/** Standalone components retain feedback when no central error UI is mounted. */
export function ErrorCenterProvider({
  store,
  children,
}: {
  store: ClientErrors;
  children: ReactNode;
}) {
  return (
    <ErrorContext.Provider value={store}>{children}</ErrorContext.Provider>
  );
}
const noSubscribe = () => () => {};
export function useLocalError(error: unknown, api = false) {
  const errors = useContext(ErrorContext);
  const hidden = useSyncExternalStore(
    errors?.subscribe ?? noSubscribe,
    () => !!errors && (api || isAPIError(error, errors)),
    () => !!errors && (api || isAPIError(error, errors)),
  );
  return !error || hidden ? "" : String(error);
}
export function useLocalErrors<T extends { error: unknown }>(
  items: readonly T[],
) {
  const errors = useContext(ErrorContext);
  const mask = () =>
    items
      .map((item) => (errors && isAPIError(item.error, errors) ? "0" : "1"))
      .join("");
  const visible = useSyncExternalStore(
    errors?.subscribe ?? noSubscribe,
    mask,
    mask,
  );
  return items.filter((_, index) => visible[index] === "1");
}
