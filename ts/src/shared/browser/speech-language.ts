// Speech services often require a regional locale, while navigator.language
// may contain a bare language such as "ko". Keep explicit user regions first.
export function speechLanguage(
  preferred: string | undefined,
  alternatives: readonly string[] = [],
): string {
  try {
    const locale = new Intl.Locale(preferred || alternatives[0] || "en-US");
    if (locale.region) return locale.baseName;
    for (const tag of alternatives) {
      try {
        const candidate = new Intl.Locale(tag);
        if (
          candidate.language === locale.language &&
          candidate.region &&
          (!locale.script || candidate.maximize().script === locale.script)
        )
          return candidate.baseName;
      } catch {
        // Ignore malformed secondary preferences, keeping the primary language.
      }
    }
    const region = locale.maximize().region;
    return [locale.language, locale.script, region].filter(Boolean).join("-");
  } catch {
    return "en-US";
  }
}

/** Use a saved override, otherwise the browser's preferred user language. */
export function browserSpeechLanguage(preferred?: string) {
  return typeof navigator === "undefined"
    ? speechLanguage(preferred)
    : speechLanguage(preferred || navigator.language, navigator.languages);
}
