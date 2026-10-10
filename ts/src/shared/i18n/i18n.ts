import { messages, type Message } from "#src/shared/i18n/locales/en.ts";

export type Locale = "en" | "ko";
export type LanguagePack = Record<Message, string>;
export const languages = [
  { id: "en", name: "English" },
  { id: "ko", name: "한국어" },
] as const;
export function resolveLocale(value: unknown): Locale {
  return value === "ko" ? "ko" : "en";
}
const loaders: Record<Exclude<Locale, "en">, () => Promise<LanguagePack>> = {
  ko: () =>
    import("#src/shared/i18n/locales/ko.ts").then((module) => module.messages),
};
type LocaleSnapshot = { locale: Locale; loading: boolean; error: string };

export class LocaleStore {
  private value: LocaleSnapshot = { locale: "en", loading: false, error: "" };
  private pack: LanguagePack = messages;
  private loaded = new Map<Locale, LanguagePack>([["en", messages]]);
  private pending = new Map<Locale, Promise<LanguagePack>>();
  private listeners = new Set<() => void>();
  private generation = 0;
  constructor(private load = loaders) {}
  snapshot = () => this.value;
  subscribe = (callback: () => void) => {
    this.listeners.add(callback);
    return () => {
      this.listeners.delete(callback);
    };
  };
  private publish(patch: Partial<LocaleSnapshot>) {
    this.value = { ...this.value, ...patch };
    this.listeners.forEach((callback) => callback());
  }
  // Preparing a pack never changes visible language or persisted settings.
  prepare(locale: Locale): Promise<LanguagePack> {
    const cached = this.loaded.get(locale);
    if (cached) return Promise.resolve(cached);
    const pending = this.pending.get(locale);
    if (pending) return pending;
    const request = this.load[locale as Exclude<Locale, "en">]()
      .then((pack) => {
        this.loaded.set(locale, pack);
        return pack;
      })
      .finally(() => this.pending.delete(locale));
    this.pending.set(locale, request);
    return request;
  }
  async activate(locale: Locale) {
    const generation = ++this.generation;
    this.publish({ loading: true, error: "" });
    try {
      const pack = await this.prepare(locale);
      if (generation !== this.generation) return;
      this.pack = pack;
      this.publish({ locale, loading: false });
    } catch (error) {
      if (generation === this.generation)
        this.publish({ loading: false, error: String(error) });
    }
  }
  translate(message: Message, values: Record<string, string | number> = {}) {
    return (this.pack[message] ?? messages[message]).replace(
      /\{(\w+)\}/g,
      (placeholder, name: string) => String(values[name] ?? placeholder),
    );
  }
}
export const localeStore = new LocaleStore();
export const t = (message: Message, values?: Record<string, string | number>) =>
  localeStore.translate(message, values);
export function translateKnown(value: string) {
  return Object.hasOwn(messages, value) ? t(value as Message) : value;
}
export const currentLocale = () => localeStore.snapshot().locale;
