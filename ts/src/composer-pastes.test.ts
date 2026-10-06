import { describe, expect, it } from "vitest";
import {
  createPaste,
  expandPastes,
  needsPasteChip,
  partialPasteEdit,
  pasteRanges,
  wholePasteSelection,
} from "./composer-pastes";

describe("composer paste chips", () => {
  it("matches TUI thresholds using Unicode characters and original UTF-8 bytes", () => {
    expect(needsPasteChip("😀".repeat(800))).toBe(false);
    expect(needsPasteChip("😀".repeat(801))).toBe(true);
    expect(needsPasteChip("one\ntwo\nthree")).toBe(false);
    expect(needsPasteChip("one\ntwo\nthree\n")).toBe(true);
    const paste = createPaste("한\r\n글\nthree\nlast", "1234abcd");
    expect(paste.lines).toBe(4);
    expect(paste.bytes).toBe(new TextEncoder().encode(paste.body).length);
    expect(paste.token).toBe(`[Paste 1234abcd · 4L · ${paste.bytes}B]`);
  });

  it("expands only known chips in one pass without changing original whitespace", () => {
    const first = createPaste(" first\r\nbody \n", "1234abcd");
    const second = createPaste(`literal ${first.token}`, "abcd1234");
    const pastes = new Map([first, second].map((p) => [p.token, p]));
    const text = `前 ${second.token}\n${first.token}${first.token} 後`;
    expect(expandPastes(text, pastes)).toBe(
      `前 ${second.body}\n${first.body}${first.body} 後`,
    );
    expect(expandPastes("[Paste 99999999 · 4L · 10B]", pastes)).toBe(
      "[Paste 99999999 · 4L · 10B]",
    );
    expect(
      pasteRanges(text, pastes).map((r) => text.slice(r.start, r.end)),
    ).toEqual([second.token, first.token, first.token]);
  });

  it("keeps selection and edits atomic, including partial Unicode-prefixed selections", () => {
    const paste = createPaste("one\ntwo\nthree\nfour", "1234abcd");
    const pastes = new Map([[paste.token, paste]]);
    const text = `😀 ${paste.token} end`;
    const [range] = pasteRanges(text, pastes);
    expect(
      wholePasteSelection([range], range.start + 1, range.end - 1),
    ).toEqual({ start: range.start, end: range.end });
    expect(
      wholePasteSelection([range], range.start + 2, range.start + 2),
    ).toEqual({ start: range.end, end: range.end });
    expect(
      partialPasteEdit(
        text,
        text.slice(0, range.start) + text.slice(range.end),
        pastes,
      ),
    ).toBe(false);
    expect(
      partialPasteEdit(
        text,
        text.slice(0, range.start + 1) + text.slice(range.end),
        pastes,
      ),
    ).toBe(true);
    expect(
      partialPasteEdit(
        text,
        text.slice(0, range.start + 3) + "x" + text.slice(range.start + 3),
        pastes,
      ),
    ).toBe(true);
    expect(partialPasteEdit(text, `${text}!`, pastes)).toBe(false);
  });
});
