import { PASTE_CHIP_PREFIX } from "./paste-chip-layout";
import { expect, test } from "vitest";
import { fileChipText } from "@lesomnus/nox";
import { createPaste, createFileChip } from "./composer-pastes";
import { presentPaste } from "./paste-presentation";
import { PasteEditHistory, PasteProjection } from "./paste-projection";

test("textarea text reserves decoration space and keeps five size cells", () => {
  const small = createPaste("x".repeat(55), "11111111");
  const large = createPaste("x".repeat(1230), "22222222");
  const pastes = new Map([
    [small.token, small],
    [large.token, large],
  ]);
  const source = `한 ${small.token} ${large.token} 뒤`;
  const projection = new PasteProjection(source, pastes);
  expect(projection.text).toBe(
    `한 ${pasteText("0055B")} ${pasteText("1.23K")} 뒤`,
  );
  expect(projection.ranges.map((r) => r.end - r.start)).toEqual([
    pasteText("0055B").length,
    pasteText("1.23K").length,
  ]);
  expect(presentPaste(small).leading).toBe("00");
  for (const range of projection.ranges) {
    expect(projection.toSource(range.start)).toBe(range.sourceStart);
    expect(projection.toSource(range.end)).toBe(range.sourceEnd);
    expect(projection.toDisplay(range.sourceStart)).toBe(range.start);
    expect(projection.toDisplay(range.sourceEnd)).toBe(range.end);
  }
  expect(projection.toSource(projection.text.length)).toBe(source.length);
});

test("identical labels retain their own bodies when either chip is deleted or edited around", () => {
  const first = createPaste("first", "11111111");
  const second = createPaste("other", "22222222");
  const pastes = new Map([
    [first.token, first],
    [second.token, second],
  ]);
  const projection = new PasteProjection(first.token + second.token, pastes);
  const label = pasteText("0005B");
  const width = label.length;
  expect(projection.text).toBe(label + label);
  expect(projection.edit(label, 0)).toBe(second.token);
  expect(projection.edit(label, width)).toBe(first.token);
  expect(projection.splice(0, width, "")).toBe(second.token);
  expect(projection.splice(width, 2 * width, "")).toBe(first.token);
  expect(projection.edit(label + "!" + label, width + 1)).toBe(
    first.token + "!" + second.token,
  );
  expect(projection.edit(pasteText("0095B") + label, 12)).toBeUndefined();
});

test("progress and failure widths map selection back to the stable reference", () => {
  const paste = createFileChip(new File(["hello"], "sample.txt"), "sample.txt");
  const pastes = new Map([[paste.token, paste]]);
  const source = paste.token + " tail";
  const pending = new PasteProjection(source, pastes);
  const caret = pending.toSource(pending.text.length);
  paste.attachment!.state = "ready";
  const ready = new PasteProjection(source, pastes);
  expect(ready.text).toBe(fileChipText("sample.txt · 0005B") + " tail");
  expect(ready.toDisplay(caret)).toBe(ready.text.length);
});

test("image progress and size share text-paste labels without exposing file references", () => {
  const image = createFileChip(
    new File(["hello"], "image.png", { type: "image/png" }),
    "image.png",
  );
  const pastes = new Map([[image.token, image]]);
  const source = image.token + " tail";
  image.attachment!.progress = 100;
  const uploading = new PasteProjection(source, pastes);
  expect(presentPaste(image)).toMatchObject({
    label: " 099%",
    progress: 99,
    before: "",
  });
  expect(uploading.text).toBe(fileChipText(" 099%") + " tail");
  image.attachment!.state = "ready";
  const ready = new PasteProjection(source, pastes);
  expect(presentPaste(image)).toMatchObject({ label: "0005B", bytes: 5 });
  expect(ready.text).toBe(fileChipText("0005B") + " tail");
  expect(ready.text.length).toBe(uploading.text.length);
  expect(ready.toSource(ready.text.length)).toBe(source.length);
  expect(ready.source).toContain("image.png");
});

test("native undo and redo restore exact references even with identical labels", () => {
  const first = createPaste("first", "11111111");
  const second = createPaste("other", "22222222");
  const pastes = new Map([
    [first.token, first],
    [second.token, second],
  ]);
  const initial = new PasteProjection(first.token + second.token, pastes);
  const history = new PasteEditHistory(initial);
  const remaining = new PasteProjection(second.token, pastes);
  history.record(remaining.source, remaining.text);
  for (let index = 1; index <= 200; index++)
    history.record(
      remaining.source + "x".repeat(index),
      remaining.text + "x".repeat(index),
    );
  const typingUndo =
    history.restore(remaining.text, "historyUndo") ?? remaining.source;
  history.record(typingUndo, remaining.text);
  expect(history.restore(initial.text, "historyUndo")).toBe(initial.source);
  expect(history.restore(remaining.text, "historyRedo")).toBe(second.token);
});

function pasteText(label: string) {
  return fileChipText(label, PASTE_CHIP_PREFIX);
}

test("measured chip reservations preserve atomic edits and frozen IME views", () => {
  const paste = createPaste("hello", "11111111");
  const pastes = new Map([[paste.token, paste]]);
  const text = "\u200b\u2007\u2063measured\u200b";
  const layouts = new Map([[paste.token, { label: "0005B", text }]]);
  const source = `앞 ${paste.token} 뒤`;
  const projection = new PasteProjection(source, pastes, undefined, layouts);
  expect(projection.text).toBe(`앞 ${text} 뒤`);
  const range = projection.ranges[0];
  expect(projection.splice(range.start, range.end, "")).toBe("앞  뒤");
  expect(projection.toSource(range.end)).toBe(range.sourceEnd);
  expect(projection.edit(`앞 ${text}! 뒤`, range.end + 1)).toBe(
    `앞 ${paste.token}! 뒤`,
  );
  const frozen = new Map([[paste.token, range.presentation]]);
  layouts.set(paste.token, { label: "0005B", text: "different geometry" });
  expect(new PasteProjection(source, pastes, frozen, layouts).text).toBe(
    projection.text,
  );
  layouts.set(paste.token, { label: "old label", text: "stale geometry" });
  expect(
    new PasteProjection(source, pastes, undefined, layouts).text,
  ).not.toContain("stale geometry");
});
