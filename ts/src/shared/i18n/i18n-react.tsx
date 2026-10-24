import { UIProvider } from "@lesomnus/nox";
import { useEffect, useSyncExternalStore, type ReactNode } from "react";
import { localeStore, resolveLocale, t, translateKnown } from "./i18n";
import { useSettings } from "#src/shared/settings/settings.ts";
import { Button } from "@lesomnus/nox";

export function useLocale() {
  return useSyncExternalStore(localeStore.subscribe, localeStore.snapshot);
}
export function LocaleProvider({ children }: { children: ReactNode }) {
  const { snapshot } = useSettings();
  const requested = resolveLocale(snapshot.document["ui.language"]);
  const { locale, error } = useLocale();
  useEffect(() => {
    void localeStore.activate(requested);
  }, [requested]);
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  return (
    <>
      {error && (
        <div className="language-error" role="alert">
          {t(
            "Unable to load the language pack. Your previous language is still active.",
          )}
          <Button onClick={() => void localeStore.activate(requested)}>
            {t("Retry")}
          </Button>
        </div>
      )}
      <UIProvider
        translate={(message, values = {}) =>
          translateKnown(message).replace(
            /\{(\w+)\}/g,
            (placeholder, name: string) => String(values[name] ?? placeholder),
          )
        }
        locale={locale}
      >
        {children}
      </UIProvider>
    </>
  );
}
