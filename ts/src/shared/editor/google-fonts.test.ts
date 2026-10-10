import { afterEach, expect, it, vi } from "vitest";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  vi.resetModules();
});
async function fixture() {
  vi.resetModules();
  const links: {
    href: string;
    onload: (() => void) | null;
    onerror: (() => void) | null;
    remove: ReturnType<typeof vi.fn>;
  }[] = [];
  let resolve!: (faces: FontFace[]) => void;
  const promise = new Promise<FontFace[]>((done) => {
    resolve = done;
  });
  const binary = { promise, resolve };
  const load = vi.fn(() => binary.promise);
  vi.stubGlobal("document", {
    createElement: () => ({
      href: "",
      onload: null,
      onerror: null,
      remove: vi.fn(),
    }),
    head: { append: (link: (typeof links)[number]) => links.push(link) },
    fonts: { load },
  });
  const api = await import("./google-fonts");
  return { ...api, links, binary, load };
}
it("deduplicates downloads and does not signal readiness until font bytes load", async () => {
  const f = await fixture();
  const first = f.loadGoogleFont("Roboto Mono");
  expect(f.loadGoogleFont("Roboto Mono")).toBe(first);
  expect(f.links).toHaveLength(1);
  expect(new URL(f.links[0].href).searchParams.get("family")).toBe(
    "Roboto Mono",
  );
  f.links[0].onload!();
  expect(f.googleFontStatus("Roboto Mono")).toBe("loading");
  expect(f.load).toHaveBeenCalledWith('400 14px "Roboto Mono"', "Aa0가");
  f.binary.resolve([{} as FontFace]);
  await first;
  expect(f.googleFontStatus("Roboto Mono")).toBe("ready");
  expect(f.loadGoogleFont("Roboto Mono")).toBe(first);
});
it("removes failed stylesheets and retries with a fresh request", async () => {
  const f = await fixture();
  const first = f.loadGoogleFont("JetBrains Mono");
  const rejected = expect(first).rejects.toThrow("stylesheet");
  f.links[0].onerror!();
  await rejected;
  expect(f.links[0].remove).toHaveBeenCalled();
  expect(f.googleFontStatus("JetBrains Mono")).toBe("error");
  const retry = f.loadGoogleFont("JetBrains Mono");
  expect(f.links).toHaveLength(2);
  f.links[1].onload!();
  f.binary.resolve([{} as FontFace]);
  await retry;
  expect(f.googleFontStatus("JetBrains Mono")).toBe("ready");
});
it("bounds a stalled font download and ignores its late completion", async () => {
  vi.useFakeTimers();
  const f = await fixture();
  const pending = f.loadGoogleFont("Noto Sans KR");
  const rejected = expect(pending).rejects.toThrow("timed out");
  f.links[0].onload!();
  await vi.advanceTimersByTimeAsync(12000);
  await rejected;
  f.binary.resolve([{} as FontFace]);
  await Promise.resolve();
  expect(f.googleFontStatus("Noto Sans KR")).toBe("error");
  expect(f.links[0].remove).toHaveBeenCalled();
});
it("rejects malformed family names before building remote requests", async () => {
  const f = await fixture();
  expect(() => f.loadGoogleFont("font&text=secret")).toThrow("Invalid");
  expect(f.links).toHaveLength(0);
});
