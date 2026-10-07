import { useEffect, useSyncExternalStore, type ReactNode } from "react";
import { localeStore, resolveLocale, t } from "./i18n";
import { useSettings } from "./settings";
import { Button } from "./button";

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
      {children}
    </>
  );
}
