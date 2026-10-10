import { describe, expect, it } from "vitest";
import {
  codeBlocks,
  composerPrompt,
  detectCodeSyntax,
  highlightCodeLines,
} from "./composer-code";
import { createPaste } from "./composer-pastes";

describe("composer code blocks", () => {
  it("recognizes line fences, multiple blocks and longer fences without treating inline ticks as blocks", () => {
    const text =
      "inline ``` is text\n```js\nconst n = 1;\n```\nafter\n ````python\n```\nprint(1)\n ````\n```\nunfinished";
    const blocks = codeBlocks(text);
    expect(
      blocks.map(({ syntax, closed, startLine, endLine }) => ({
        syntax,
        closed,
        startLine,
        endLine,
      })),
    ).toEqual([
      { syntax: "js", closed: true, startLine: 1, endLine: 3 },
      { syntax: "python", closed: true, startLine: 5, endLine: 8 },
      { syntax: "auto", closed: false, startLine: 9, endLine: 10 },
    ]);
    expect(text.slice(blocks[1].bodyStart, blocks[1].bodyEnd)).toBe(
      "```\nprint(1)\n",
    );
  });

  it("detects syntax at send, canonicalizes aliases, closes unfinished fences and preserves surrounding/body text", () => {
    const json = '{"hello": true, "count": 42}';
    expect(detectCodeSyntax(json)).toBe("json");
    expect(
      composerPrompt(`before\n\`\`\`\n${json}\n\`\`\`\nafter`, new Map()),
    ).toBe(`before\n\`\`\`json\n${json}\n\`\`\`\nafter`);
    expect(composerPrompt("```py\nprint('hi')", new Map())).toBe(
      "```python\nprint('hi')\n```",
    );
    expect(composerPrompt("```auto\n", new Map())).toBe("```plaintext\n```");
    expect(composerPrompt("plain\r\ntext", new Map())).toBe("plain\r\ntext");
    expect(composerPrompt("```custom extra\nraw\n```", new Map())).toBe(
      "```custom extra\nraw\n```",
    );
    expect(composerPrompt("````md\n```\n````", new Map())).toBe(
      "````markdown\n```\n````",
    );
  });

  it("detects expanded paste content and sends code with the exact original paste body", () => {
    const paste = createPaste(
      '{\r\n  "hello": true,\r\n  "count": 42\r\n}',
      "1234abcd",
    );
    const pastes = new Map([[paste.token, paste]]);
    expect(
      composerPrompt(`prefix\n\`\`\`\n${paste.token}\n\`\`\`\nsuffix`, pastes),
    ).toBe(`prefix\n\`\`\`json\n${paste.body}\n\`\`\`\nsuffix`);
    const nested = createPaste("```python\nprint('hi')\n```", "abcd1234");
    pastes.set(nested.token, nested);
    expect(composerPrompt(`before ${nested.token} after`, pastes)).toBe(
      `before ${nested.body} after`,
    );
    expect(
      composerPrompt(`\`\`\`plaintext\n${nested.token}\n\`\`\``, pastes),
    ).toBe(`\`\`\`\`plaintext\n${nested.body}\n\`\`\`\``);
    const innerLabel = createPaste(nested.token, "abcd9876");
    pastes.set(innerLabel.token, innerLabel);
    expect(composerPrompt(innerLabel.token, pastes)).toBe(nested.token);
    expect(
      composerPrompt("```js\r\nlet a = 1;\r\n```\r\nafter", new Map()),
    ).toBe("```javascript\r\nlet a = 1;\r\n```\r\nafter");
  });

  it("preserves every character through multiline highlighting and renders user HTML as text", () => {
    for (const body of [
      "/* multiple\n lines */\nconst value = '<img src=x onerror=alert(1)> & \\\"';\n",
      "const x = `a\nb`;\n'&lt;'",
      "",
      "\n\n",
      "한글 😀",
    ]) {
      const lines = highlightCodeLines(body, "javascript");
      expect(
        lines
          .map((line) => line.map((token) => token.text).join(""))
          .join("\n"),
      ).toBe(body);
    }
    expect(highlightCodeLines("raw <tag>", "unknown")).toEqual([
      [{ text: "raw <tag>", className: "" }],
    ]);
    expect(
      highlightCodeLines("const n = 1;", "javascript")[0].some((token) =>
        token.className.includes("hljs-keyword"),
      ),
    ).toBe(true);
    expect(highlightCodeLines("x".repeat(65537), "javascript")[0]).toEqual([
      { text: "x".repeat(65537), className: "" },
    ]);
    expect(detectCodeSyntax("a".repeat(1024 * 1024))).toBe("plaintext");
    expect(highlightCodeLines("a".repeat(4096), "cpp")[0]).toEqual([
      { text: "a".repeat(4096), className: "" },
    ]);
  });
});
