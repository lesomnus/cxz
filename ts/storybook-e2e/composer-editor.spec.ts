import { expect, test } from "@playwright/test";

const sentence =
  "composer toolbar에서 터미널 버튼 왼쪽에 메뉴 버튼 추가해줘. 메뉴에는 세션 상태 관리 그룹: 정지(or 재개), 재시작, purge 버튼을 추가해줘. 세션 정보 상세보기 기 ";

test("plain Korean drafts keep native text and caret together during composition", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-playground--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const editor = page.locator(".composer-editor");
  const cdp = await page.context().newCDPSession(page);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    for (const zoom of [1, 1.1]) {
      await page.evaluate((zoom) => {
        document.documentElement.style.zoom = String(zoom);
      }, zoom);
      await input.fill(sentence);
      await expect(input).not.toHaveCSS("color", "rgba(0, 0, 0, 0)");
      await expect(page.locator(".editor-surface")).toHaveCSS(
        "visibility",
        "hidden",
      );
      const paint = () =>
        input.evaluate((el) => {
          const style = getComputedStyle(el);
          return {
            color: style.color,
            font: style.font,
            width: el.clientWidth,
            height: el.clientHeight,
          };
        });
      await input.press("End");
      await input.press("Home");
      const pastedHome = await input.evaluate(
        (el: HTMLTextAreaElement) => el.selectionStart,
      );
      const before = await paint();
      const prefix = sentence.slice(0, -2);
      await input.fill(prefix);
      await input.evaluate((el: HTMLTextAreaElement) =>
        el.setSelectionRange(el.value.length, el.value.length),
      );
      await cdp.send("Input.imeSetComposition", {
        text: "기",
        selectionStart: 1,
        selectionEnd: 1,
      });
      await expect(editor).toHaveAttribute("data-composing", "true");
      await expect(input).toHaveValue(`${prefix}기`);
      expect(await paint()).toEqual(before);
      await expect(page.locator(".editor-surface")).toHaveCSS(
        "visibility",
        "hidden",
      );
      await cdp.send("Input.insertText", { text: "기" });
      await expect(editor).toHaveAttribute("data-composing", "false");
      await input.press("Space");
      await expect(input).toHaveValue(sentence);
      expect(await paint()).toEqual(before);
      await input.press("Home");
      expect(
        await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
      ).toBe(pastedHome);
    }
  }
});

test("zoom does not inflate editor expansion or line-number spacing", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--empty&viewMode=story",
  );
  await page.evaluate(() => {
    document.documentElement.style.zoom = "1.1";
  });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("한글 first\n한글 second\n한글 third\n한글 fourth");
  await expect(input).toHaveAttribute("data-expanded", "false");
  await expect
    .poll(() =>
      page.evaluate(() => {
        const numbers = [
          ...document.querySelectorAll(".editor-gutter > div > div"),
        ];
        const lines = [...document.querySelectorAll(".editor-line")];
        return Math.max(
          ...numbers.map((number, index) =>
            Math.abs(
              number.getBoundingClientRect().top -
                lines[index].getBoundingClientRect().top,
            ),
          ),
        );
      }),
    )
    .toBeLessThan(1);
});
