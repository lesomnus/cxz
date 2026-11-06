import {
  createContext,
  useContext,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { isAPIError, type ClientErrors } from "./client-errors";

const ErrorContext = createContext<ClientErrors | null>(null);
const LocalErrorsContext = createContext(false);
export const useErrorCenter = () => useContext(ErrorContext);
/** Standalone components retain feedback when no central error UI is mounted. */
export function ErrorCenterProvider({
  store,
  localErrors = false,
  children,
}: {
  store: ClientErrors;
  localErrors?: boolean;
  children: ReactNode;
}) {
  return (
    <ErrorContext.Provider value={store}>
      <LocalErrorsContext.Provider value={localErrors}>
        {children}
      </LocalErrorsContext.Provider>
    </ErrorContext.Provider>
  );
}
const noSubscribe = () => () => {};
export function useLocalError(error: unknown, api = false) {
  const errors = useContext(ErrorContext);
  const localErrors = useContext(LocalErrorsContext);
  const hidden = useSyncExternalStore(
    errors?.subscribe ?? noSubscribe,
    () => !localErrors && !!errors && (api || isAPIError(error, errors)),
    () => !localErrors && !!errors && (api || isAPIError(error, errors)),
  );
  return !error || hidden ? "" : String(error);
}
export function useLocalErrors<T extends { error: unknown }>(
  items: readonly T[],
) {
  const errors = useContext(ErrorContext);
  const localErrors = useContext(LocalErrorsContext);
  const mask = () =>
    items
      .map((item) =>
        !localErrors && errors && isAPIError(item.error, errors) ? "0" : "1",
      )
      .join("");
  const visible = useSyncExternalStore(
    errors?.subscribe ?? noSubscribe,
    mask,
    mask,
  );
  return items.filter((_, index) => visible[index] === "1");
}
