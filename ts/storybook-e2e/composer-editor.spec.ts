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

// Read soft-wrap starts using the browser's own textarea navigation. A clone
// avoids the editor's intentional selection snapping inside attachment chips.
async function wrapping(
  input: import("@playwright/test").Locator,
  page: import("@playwright/test").Page,
) {
  const mirrored = await input.evaluate((el: HTMLTextAreaElement) => {
    const mirror = el
      .closest(".composer-editor")!
      .querySelector(".editor-mirror")!;
    const starts: number[] = [];
    let offset = 0;
    for (const line of mirror.children) {
      const walker = document.createTreeWalker(line, NodeFilter.SHOW_TEXT);
      let node: Node | null, lastTop: number | undefined;
      while ((node = walker.nextNode())) {
        if (node.parentElement?.closest("[data-file-chip-control]")) continue;
        for (let i = 0; i < node.textContent!.length; i++) {
          const range = document.createRange();
          range.setStart(node, i);
          range.setEnd(node, i + 1);
          const top = range.getBoundingClientRect().top;
          if (top !== lastTop) starts.push(offset);
          lastTop = top;
          offset++;
        }
      }
      offset++; // newlines separate the mirror's block elements
    }
    const native = el.cloneNode() as HTMLTextAreaElement;
    native.removeAttribute("id");
    native.dataset.nativeProbe = "true";
    native.value = el.value;
    Object.assign(native.style, {
      position: "absolute",
      top: "0",
      left: "0",
      opacity: "0",
      pointerEvents: "none",
      transition: "none",
    });
    el.parentElement!.append(native);
    return starts;
  });
  const native = page.locator("[data-native-probe]");
  try {
    await native.press("Control+End");
    const starts: number[] = [];
    for (let i = 0; i < 40; i++) {
      await native.press("Home");
      const start = await native.evaluate(
        (el: HTMLTextAreaElement) => el.selectionStart,
      );
      if (starts.includes(start)) break;
      starts.push(start);
      if (!start) break;
      await native.press("ArrowUp");
    }
    return { native: starts.reverse(), mirrored };
  } finally {
    await native.evaluate((el) => el.remove());
  }
}

test("decorated Korean drafts match native wrapping at fractional widths while typing and composing", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-playground--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  // Synthetic attachment reference: no user file contents or secrets are needed.
  const prefix =
    "토큰: `[Attached file: /cxz/assets/aaaaaaaaaaaaaaaaaaaaaaaa/bbbbbbbbbbbbbbbbbbbbbbbb/paste-cccccccc.txt — read this file for the full content]`\n";
  const lastLine =
    "근데 한도를 0으로 안한 이유가, OOM 상황에서 OOM kill 순위가 가장 낮아져서 정작 중요한 app이";
  const cdp = await page.context().newCDPSession(page);
  for (const width of [390, 590, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    for (const zoom of [1, 1.1, 1.25]) {
      await page.evaluate((zoom) => {
        document.documentElement.style.zoom = String(zoom);
      }, zoom);
      await input.fill(prefix + lastLine);
      await expect(page.locator(".composer-editor")).toHaveAttribute(
        "data-decorated",
        "true",
      );
      for (const tail of ["", "고 있는거지?"]) {
        await input.press("Control+End");
        if (tail) await input.pressSequentially(tail);
        const rows = await wrapping(input, page);
        expect(
          rows.mirrored,
          `width=${width}, zoom=${zoom}, tail=${tail}`,
        ).toEqual(rows.native);
      }
      await input.fill(prefix + lastLine);
      await input.press("Control+End");
      await cdp.send("Input.imeSetComposition", {
        text: "요",
        selectionStart: 1,
        selectionEnd: 1,
      });
      await expect(page.locator(".composer-editor")).toHaveAttribute(
        "data-composing",
        "true",
      );
      await expect(input).not.toHaveCSS("color", "rgba(0, 0, 0, 0)");
      await cdp.send("Input.insertText", { text: "요" });
      await expect(page.locator(".composer-editor")).toHaveAttribute(
        "data-composing",
        "false",
      );
      const rows = await wrapping(input, page);
      expect(rows.mirrored).toEqual(rows.native);
    }
  }
});

test("a pasted chip and the following Korean paragraph keep the same native line breaks", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-playground--interactive&viewMode=story",
  );
  await page.setViewportSize({ width: 390, height: 900 });
  await page.evaluate(() => {
    document.documentElement.style.zoom = "1.1";
  });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("토큰: `");
  await input.evaluate((el) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", "synthetic attachment\n".repeat(100));
    el.dispatchEvent(
      new ClipboardEvent("paste", {
        bubbles: true,
        cancelable: true,
        clipboardData,
      }),
    );
  });
  await expect(page.locator(".paste-chip")).toBeVisible();
  await input.press("Control+End");
  // Add the rest without replacing the chip or bypassing its native edit path.
  await input.press("`");
  await input.press("Enter");
  await input.pressSequentially(
    "근데 한도를 0으로 안한 이유가, OOM 상황에서 OOM kill 순위가 가장 낮아져서 정작 중요한 app이",
  );
  const rows = await wrapping(input, page);
  expect(rows.mirrored).toEqual(rows.native);
});

test("chips and code controls remain visible throughout Korean composition", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-playground--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("```typescript\nconst value = 1;\n```\n");
  await input.evaluate((node: HTMLTextAreaElement) => {
    node.focus();
    node.setSelectionRange(node.value.length, node.value.length);
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", "synthetic paste\n".repeat(20));
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await input.press("Control+End");
  await input.press("Enter");
  const cdp = await page.context().newCDPSession(page);
  for (const text of ["ㅎ", "하", "한"]) {
    await cdp.send("Input.imeSetComposition", {
      text,
      selectionStart: 1,
      selectionEnd: 1,
    });
    await expect(page.locator(".composer-editor")).toHaveAttribute(
      "data-composing",
      "true",
    );
    await expect(page.locator(".paste-chip-label")).toBeVisible();
    await expect(page.locator(".paste-chip-token")).toHaveCount(0);
    await expect(page.locator(".editor-code-controls select")).toBeVisible();
    await expect(
      page.locator(".editor-code-controls .code-close"),
    ).toBeVisible();
  }
  await cdp.send("Input.insertText", { text: "한" });
  await expect(page.locator(".composer-editor")).toHaveAttribute(
    "data-composing",
    "false",
  );
  await expect(page.locator(".paste-chip-label")).toBeVisible();
  await expect(page.locator(".editor-code-controls select")).toBeVisible();
  await expect(input).toHaveValue(/\n한$/);
});

test("the native caret remains visible immediately after a paste chip", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-playground--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.evaluate((node) => {
    node.focus();
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", "synthetic paste\n".repeat(20));
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await input.press("Control+End");
  const chip = page.locator(".paste-chip");
  const bounds = (await chip.boundingBox())!;
  let visible = false;
  for (let attempt = 0; attempt < 5 && !visible; attempt++) {
    await input.evaluate((node) => {
      node.blur();
      node.focus();
    });
    const image = await page.screenshot({
      caret: "initial",
      clip: {
        x: Math.floor(bounds.x + bounds.width) - 3,
        y: Math.floor(bounds.y),
        width: 6,
        height: Math.ceil(bounds.height),
      },
    });
    const count = await page.evaluate(async (base64) => {
      const image = new Image();
      image.src = `data:image/png;base64,${base64}`;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = image.width;
      canvas.height = image.height;
      const context = canvas.getContext("2d")!;
      context.drawImage(image, 0, 0);
      const pixels = context.getImageData(0, 0, image.width, image.height).data;
      let count = 0;
      for (let offset = 0; offset < pixels.length; offset += 4)
        if (
          pixels[offset] > 180 &&
          pixels[offset + 1] > 180 &&
          pixels[offset + 2] > 180
        )
          count++;
      return count;
    }, image.toString("base64"));
    visible = count >= 6;
  }
  expect(visible).toBe(true);
});
