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
    `한 ${fileChipText("[Text 0055B]")} ${fileChipText("[Text 1.23K]")} 뒤`,
  );
  expect(projection.ranges.map((r) => r.end - r.start)).toEqual([
    fileChipText("[Text 0055B]").length,
    fileChipText("[Text 1.23K]").length,
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
  const label = fileChipText("[Text 0005B]");
  const width = label.length;
  expect(projection.text).toBe(label + label);
  expect(projection.edit(label, 0)).toBe(second.token);
  expect(projection.edit(label, width)).toBe(first.token);
  expect(projection.splice(0, width, "")).toBe(second.token);
  expect(projection.splice(width, 2 * width, "")).toBe(first.token);
  expect(projection.edit(label + "!" + label, width + 1)).toBe(
    first.token + "!" + second.token,
  );
  expect(
    projection.edit(fileChipText("[Text 0095B]") + label, 12),
  ).toBeUndefined();
});

test("progress and failure widths map selection back to the stable reference", () => {
  const paste = createFileChip(new File(["hello"], "sample.txt"), "sample.txt");
  const pastes = new Map([[paste.token, paste]]);
  const source = paste.token + " tail";
  const pending = new PasteProjection(source, pastes);
  const caret = pending.toSource(pending.text.length);
  paste.attachment!.state = "ready";
  const ready = new PasteProjection(source, pastes);
  expect(ready.text).toBe(fileChipText("sample.txt · 5 B") + " tail");
  expect(ready.toDisplay(caret)).toBe(ready.text.length);
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
