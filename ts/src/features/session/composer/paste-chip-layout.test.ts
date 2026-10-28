import { expect, test } from "vitest";
import { pasteExcerpt } from "./paste-chip-layout";

test("paste previews contain at most ten intact characters and flatten line breaks", () => {
  expect(pasteExcerpt("1234567890more")).toBe("1234567890…");
  expect(pasteExcerpt("1234567890")).toBe("1234567890");
  expect(pasteExcerpt("a\r\nb\nc\rd\u2028e\u2029")).toBe("a b c d e ");
  expect(pasteExcerpt("가나다라마바사아자차카")).toBe("가나다라마바사아자차…");
  expect(pasteExcerpt("e\u0301👨‍👩‍👧‍👦🇰🇷12345678")).toBe("e\u0301👨‍👩‍👧‍👦🇰🇷1234567…");
});
