import { expect, it } from "vitest";
import { conversationMarkdown } from "./markdown-parser";

it("keeps single and doubled tildes literal, including Korean ranges", () => {
  const html = conversationMarkdown.parse("~text~ / ~~text~~ / 1~3일, 4~6일", {
    async: false,
  });
  expect(html).toContain("~text~ / ~~text~~ / 1~3일, 4~6일");
  expect(html).not.toContain("<del>");
});
it("keeps other Markdown formatting and code unchanged", () => {
  const html = conversationMarkdown.parse(
    "**bold** and ~~*italic*~~\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n~~~txt\n~~code~~\n~~~",
    { async: false },
  );
  expect(html).toContain("<strong>bold</strong>");
  expect(html).toContain("~~<em>italic</em>~~");
  expect(html).toContain("<table>");
  expect(html).toContain('class="language-txt">~~code~~');
  expect(html).not.toContain("<del>");
});
