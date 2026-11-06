import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { controlCharacters } from "./control-characters";

test("control markers ignore chip geometry while preserving surrounding whitespace", () => {
  const reservation = "\u200b\u00a0\u2060\u2007\u2063\u200b";
  const line = ` ${reservation} after`;
  const html = renderToStaticMarkup(
    <>{controlCharacters(line, [{ start: 1, end: reservation.length + 1 }])}</>,
  );
  expect(html.match(/data-symbol=/g)).toHaveLength(2);
  expect(html).not.toContain('data-symbol="¦"');
  expect(html).toContain(reservation);
});

test("visible spaces, tabs and newlines do not replace the original text", () => {
  const original = "alpha beta\tgamma\n";
  const html = renderToStaticMarkup(<>{controlCharacters(original)}</>);
  expect(html).toContain('data-symbol="·"');
  expect(html).toContain('data-symbol="→"');
  expect(html).toContain('data-symbol="¶"');
  expect(html.replace(/<[^>]*>/g, "")).toBe(original);
});
