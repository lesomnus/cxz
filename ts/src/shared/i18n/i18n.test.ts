import { describe, expect, it, vi } from "vitest";
import { LocaleStore, resolveLocale } from "./i18n";
import { messages as en } from "#src/shared/i18n/locales/en.ts";
import { messages as ko } from "#src/shared/i18n/locales/ko.ts";
import { parseSettings } from "#src/shared/settings/editor-settings.ts";

const deferred = () => {
  let resolve!: (value: typeof ko) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<typeof ko>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
};
describe("language packs", () => {
  it("defaults to English without fetching and only prepares a pack on demand", async () => {
    const request = deferred();
    const load = vi.fn(() => request.promise);
    const store = new LocaleStore({ ko: load });
    expect(store.translate("Settings")).toBe("Settings");
    await store.prepare("en");
    expect(load).not.toHaveBeenCalled();
    const first = store.prepare("ko");
    const second = store.prepare("ko");
    expect(load).toHaveBeenCalledTimes(1);
    expect(store.translate("Settings")).toBe("Settings");
    request.resolve(ko);
    await Promise.all([first, second]);
    expect(store.snapshot().locale).toBe("en");
    await store.activate("ko");
    expect(store.translate("Settings")).toBe("설정");
    expect(
      store.translate("Paste · {lines} lines · {bytes}B", {
        lines: 3,
        bytes: 42,
      }),
    ).toBe("붙여넣기 · 3줄 · 42B");
    expect(load).toHaveBeenCalledTimes(1);
  });
  it("keeps the active language on failure and allows a failed load to retry", async () => {
    const load = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue(ko);
    const store = new LocaleStore({ ko: load });
    await store.activate("ko");
    expect(store.snapshot()).toMatchObject({
      locale: "en",
      loading: false,
      error: "Error: offline",
    });
    await store.activate("ko");
    expect(store.snapshot()).toEqual({
      locale: "ko",
      loading: false,
      error: "",
    });
  });
  it("discards a late language load after a newer preference wins", async () => {
    const request = deferred();
    const store = new LocaleStore({ ko: () => request.promise });
    const old = store.activate("ko");
    await store.activate("en");
    request.resolve(ko);
    await old;
    expect(store.snapshot().locale).toBe("en");
  });
  it("validates the single-file preference and keeps complete matching placeholders", () => {
    expect(resolveLocale(undefined)).toBe("en");
    expect(parseSettings('{"ui.language":"ko","future":true}')).toEqual({
      "ui.language": "ko",
      future: true,
    });
    for (const value of ['"fr"', "null", "1", "false"])
      expect(() => parseSettings(`{"ui.language":${value}}`)).toThrow(
        "Unsupported display language",
      );
    const placeholders = (text: string) =>
      [...text.matchAll(/\{\w+\}/g)].map((x) => x[0]).sort();
    expect(Object.keys(ko).sort()).toEqual(Object.keys(en).sort());
    for (const key of Object.keys(en) as (keyof typeof en)[]) {
      expect(en[key]).not.toMatch(/[가-힣]/);
      expect(placeholders(ko[key]), key).toEqual(placeholders(en[key]));
    }
  });
});
