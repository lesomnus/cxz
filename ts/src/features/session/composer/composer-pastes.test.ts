import { describe, expect, it } from "vitest";
import {
  createPaste,
  expandPastes,
  needsPasteChip,
  partialPasteEdit,
  pasteRanges,
  wholePasteSelection,
  attachmentsReady,
  defaultPasteSendMode,
} from "./composer-pastes";

describe("composer paste chips", () => {
  it("sends the selected inline body or file path and gates only file delivery", () => {
    const paste = createPaste("original\r\ntext", "1234abcd");
    paste.attachment = { name: "paste.txt", state: "uploading" };
    const pastes = new Map([[paste.token, paste]]);
    expect(attachmentsReady(paste.token, pastes)).toBe(false);
    paste.sendAs = "text";
    expect(attachmentsReady(paste.token, pastes)).toBe(true);
    expect(expandPastes(paste.token, pastes)).toBe(paste.body);
    paste.attachment.path = "/cxz/assets/paste.txt";
    paste.attachment.state = "ready";
    expect(expandPastes(paste.token, pastes)).toBe(paste.body);
    paste.sendAs = "file";
    expect(attachmentsReady(paste.token, pastes)).toBe(true);
    expect(expandPastes(paste.token, pastes)).toBe(
      "[Attached file: /cxz/assets/paste.txt — read this file for the full content]",
    );
    expect(expandPastes(paste.token, pastes, false)).toBe(paste.body);
    paste.attachment.state = "error";
    expect(attachmentsReady(paste.token, pastes)).toBe(false);
    paste.sendAs = "text";
    expect(attachmentsReady(paste.token, pastes)).toBe(true);
  });
  it("keeps short pastes inline and chooses chip delivery using Unicode characters", () => {
    for (const character of ["a", "한", "😀"]) {
      expect(needsPasteChip(character.repeat(32))).toBe(false);
      expect(needsPasteChip(character.repeat(33))).toBe(true);
      expect(defaultPasteSendMode(character.repeat(33))).toBe("text");
      expect(defaultPasteSendMode(character.repeat(64))).toBe("text");
      expect(defaultPasteSendMode(character.repeat(65))).toBe("file");
    }
    expect(needsPasteChip("one\ntwo\nthree")).toBe(false);
    expect(needsPasteChip("one\ntwo\nthree\n")).toBe(false);
    expect(needsPasteChip("\n".repeat(32))).toBe(false);
    expect(needsPasteChip("\n".repeat(33))).toBe(true);
  });

  it("preserves the original UTF-8 bytes and line count", () => {
    const paste = createPaste("한\r\n글\nthree\nlast", "1234abcd");
    expect(paste.lines).toBe(4);
    expect(paste.bytes).toBe(new TextEncoder().encode(paste.body).length);
    expect(paste.token).toBe("[Paste 1234abcd]");
  });

  it("still recognizes existing saved paste tokens", () => {
    const paste = {
      ...createPaste("original text", "1234abcd"),
      token: "[Paste 1234abcd · 1L · 13B]",
    };
    const pastes = new Map([[paste.token, paste]]);
    expect(expandPastes(`Read ${paste.token}`, pastes)).toBe(
      "Read original text",
    );
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
