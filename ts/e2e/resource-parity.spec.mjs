import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { pickSearchDate } from "../test-support/search-dates.ts";
import { projectFixture } from "../test-support/projects.ts";
import { expectChipDisplay } from "../test-support/chip-display.ts";
import { expectAPIError } from "../test-support/client-errors.ts";
test.use({
  viewport: { width: 1280, height: 900 },
  isMobile: false,
  hasTouch: false,
});
function stream(rows) {
  const frames = rows.map((row) => {
    const body = Buffer.from(JSON.stringify(row));
    const header = Buffer.alloc(5);
    header.writeUInt32BE(body.length, 1);
    return Buffer.concat([header, body]);
  });
  return Buffer.concat([...frames, Buffer.from([2, 0, 0, 0, 2, 123, 125])]);
}
async function fixture(
  page,
  helpers = false,
  extraProject = false,
  initialPath = "/",
  projectAlias = "project",
) {
  const id = (value) => Buffer.from(value.padEnd(16, "0")).toString("base64");
  const project = {
    id: projectFixture().id,
    alias: projectAlias,
    name: "Demo project",
    workspace: "/workspace",
    status: { containerState: "CONTAINER_STATE_RUNNING" },
  };
  const projects = extraProject
    ? [
        project,
        {
          ...project,
          id: projectFixture(2).id,
          alias: "docs-playground",
          name: "Docs playground",
        },
      ]
    : [project];
  const account = {
    id: id("account"),
    alias: "work",
    name: "Work account",
    agent: "codex",
    authBackend: "brokered-access-token",
  };
  let queued = "Queued review request";
  let erased = false;
  const session = () => ({
    id: id("fixture"),
    runtimeId: "fixture",
    alias: "oak",
    name: "Research session",
    clientId: "creation",
    agent: "codex",
    project,
    account,
    status: { state: "idle", runId: "run", lastSeq: "4", pending: [], queued },
  });
  const memory = {
    id: id("memory"),
    name: "Research notes",
    ownerSession: memoryFixtureUUID("fixture"),
    project,
    revision: "1",
    labels: { topic: "design" },
    dateCreated: "2026-11-02T00:00:00Z",
    dateUpdated: "2026-11-02T00:00:00Z",
  };
  const handoff = {
    ...memory,
    id: id("handoff"),
    name: "Handoff notes",
    ownerSession: "",
  };
  let content = "# Shared notes\nOriginal content";
  const events = [
    {
      kind: "input",
      seq: "1",
      text: "Earlier question",
      runId: "run",
      timeMs: "1710000000000",
    },
    {
      kind: "assistant",
      seq: "2",
      text: "Earlier response",
      runId: "run",
      timeMs: "1710000001000",
    },
    {
      kind: "input",
      seq: "3",
      text: "Current question",
      runId: "run",
      timeMs: "1710000010000",
    },
    {
      kind: "assistant",
      seq: "4",
      text: "Current response",
      runId: "run",
      timeMs: "1710000011000",
    },
  ];
  const requests = [];
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (
      route.request().method() !== "GET" ||
      (url.pathname !== "/" && !url.pathname.startsWith("/assets/"))
    )
      return route.continue();
    const file = resolve(
      "../internal/webui/assets",
      url.pathname === "/" ? "index.html" : url.pathname.slice(1),
    );
    const contentType = file.endsWith(".js")
      ? "application/javascript"
      : file.endsWith(".css")
        ? "text/css"
        : file.endsWith(".html")
          ? "text/html"
          : "application/octet-stream";
    await route.fulfill({ body: readFileSync(file), contentType });
  });
  await page.route("**/cxz.*/*", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const raw = route.request().postDataBuffer();
    const q = raw?.length
      ? JSON.parse((raw[0] === 0 ? raw.subarray(5) : raw).toString())
      : {};
    requests.push({ path, q });
    const send = (json) => route.fulfill({ json });
    const sendStream = (rows) =>
      route.fulfill({
        contentType: "application/connect+json",
        body: stream(rows),
      });
    const error = (code, message) =>
      route.fulfill({ status: 409, json: { code, message } });
    const aux = {
      preferences: [{ kind: "AUX_KIND_SUMMARY", enabled: true }],
      recent: [],
      summaries: [],
    };
    if (path === "/cxz.ProjectService/List") return send({ items: projects });
    if (path === "/cxz.ProjectService/Watch")
      return sendStream([
        { items: projects.map((value) => ({ id: value.id, value })) },
      ]);
    if (path === "/cxz.ProjectService/Get") return send(project);
    if (path === "/cxz.AccountService/List") return send({ items: [account] });
    if (path === "/cxz.AccountService/Add")
      return send({ ...account, alias: q.alias, name: q.name, agent: q.agent });
    if (path === "/cxz.AuthBindingService/Add")
      return send({
        id: id("binding"),
        bindingId: "binding",
        account,
        project,
      });
    if (path === "/cxz.SessionService/List")
      return send({ items: erased ? [] : [session()] });
    if (path === "/cxz.SessionService/Watch")
      return sendStream([
        {
          items: erased
            ? []
            : [
                { id: session().id, value: session() },
                ...(helpers
                  ? [
                      {
                        id: id("helper"),
                        value: {
                          ...session(),
                          id: id("helper"),
                          runtimeId: "helper",
                          alias: "elm",
                          name: "Helper session",
                        },
                      },
                    ]
                  : []),
              ],
        },
      ]);
    if (path === "/cxz.SessionService/Get") return send(session());
    if (path === "/cxz.SessionService/Add") return send(session());
    if (path === "/cxz.SessionService/Erase") {
      erased = true;
      return send({ erased: true });
    }
    if (path === "/cxz.SessionService/Transcript")
      return send({
        events: events.filter(
          (e) => !q.afterSeq || Number(e.seq) > Number(q.afterSeq),
        ),
        snapshotSeq: "4",
      });
    if (path === "/cxz.SessionService/History")
      return send({
        events: events.filter((e) => Number(e.seq) > Number(q.afterSeq ?? 0)),
      });
    if (path === "/cxz.SessionService/Events") return sendStream([]);
    if (path === "/cxz.SessionService/AuxEvents") return sendStream([aux]);
    if (path === "/cxz.SessionService/AuxStatus") return send(aux);
    if (path === "/cxz.SessionService/AuxRun")
      return send({
        ...aux,
        current: {
          id: "aux1",
          state: "running",
          kinds: q.kinds,
          results: [],
          usage: [],
          turn: "4",
          responseSeq: "4",
        },
      });
    if (path === "/cxz.SessionService/AuxPrefer")
      return send({ ...aux, preferences: q.preferences });
    if (path === "/cxz.SessionService/AuxCancel")
      return send({
        ...aux,
        current: { id: "aux1", state: "canceled", results: [], usage: [] },
      });
    if (path === "/cxz.ProjectService/AuxConfig")
      return send({
        profiles: [
          {
            kind: "AUX_KIND_SUMMARY",
            enabled: true,
            account: "work",
            model: "test-model",
          },
        ],
      });
    if (path === "/cxz.ProjectService/AuxModels")
      return send({
        models: [
          { id: "test-model", name: "Test model", efforts: ["low", "high"] },
        ],
      });
    if (path === "/cxz.ProjectService/AuxSetConfig")
      return send({ profiles: q.profiles });
    if (path === "/cxz.SessionService/Models")
      return send({
        data: Buffer.from(JSON.stringify({ models: [] })).toString("base64"),
      });
    if (path === "/cxz.SessionService/Send") {
      if (q.cancel) queued = "";
      return send({ status: "accepted" });
    }
    if (path === "/cxz.SessionService/Search")
      return sendStream([
        {
          visit: {
            sessionId: "fixture",
            alias: "oak",
            title: "Research session",
            projectName: "Demo project",
            hits: [
              {
                seq: "2",
                kind: "assistant",
                timeMs: "1710000001000",
                snippet: "Earlier response",
              },
            ],
          },
        },
        { summary: { hits: 1, sessions: 1, projects: 1 } },
      ]);
    if (path === "/cxz.MemoryService/List")
      return send({
        items:
          q.scope === "MEMORY_SCOPE_TRASH"
            ? []
            : [memory, handoff].filter(
                (m) =>
                  (!q.project?.id || m.project.id === q.project.id) &&
                  (!q.session?.id ||
                    (q.session.id === id("fixture") &&
                      m.ownerSession === memoryFixtureUUID("fixture"))),
              ),
      });
    if (path === "/cxz.MemoryService/Get") return send(memory);
    if (path === "/cxz.MemoryService/Read") {
      const selected = [memory, handoff].find(
        (m) => m.id === q.ref?.id && m.project.id === q.ref?.project?.id,
      );
      if (!selected) return error("not_found", "Memory not found");
      return send({
        memory: selected,
        content: selected === memory ? content : "# Handoff\nNext steps",
        revision: selected.revision,
        format: "MEMORY_READ_FORMAT_FULL",
      });
    }
    if (path === "/cxz.MemoryService/Patch") {
      if (q.content?.includes("conflict")) {
        memory.revision = "2";
        memory.dateUpdated = "2026-11-02T00:00:01Z";
        content = "# Shared notes\nExternal version";
        return error("aborted", "Document changed. Read the current revision.");
      }
      if (q.content !== undefined) {
        content = q.content;
        memory.revision = "3";
      }
      return send(memory);
    }
    if (path === "/cxz.SessionService/ConversationStats")
      return send({
        complete: true,
        total: {
          finishedTurns: "2",
          inputTokens: "100",
          outputTokens: "50",
          tokenReportedTurns: "2",
        },
      });
    if (path === "/cxz.SessionService/Background")
      return send({
        data: Buffer.from(
          JSON.stringify({
            run: {
              Tasks: {
                one: {
                  task_id: "one",
                  description: "Build assets",
                  status: "working",
                  active: true,
                },
              },
            },
          }),
        ).toString("base64"),
      });
    if (path === "/cxz.ProjectService/PutSecretFile")
      return send({ path: "/run/cxz-secrets/test" });
    if (path === "/cxz.ProjectService/DeleteSecretFile")
      return send({ path: q.path });
    return send({});
  });
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto(initialPath);
  return requests;
}
test("every resource tab has its own body and navigation", async ({ page }) => {
  await fixture(page);
  for (const [name, body] of [
    [
      "Projects view",
      page.getByRole("combobox", { name: "Select project", exact: true }),
    ],
    [
      "Accounts",
      page.getByRole("article", { name: "Work account", exact: true }),
    ],
    [
      "Search",
      page.getByRole("main", { name: "Search workspace", exact: true }),
    ],
    [
      "Memory",
      page.getByRole("main", { name: "Shared memory workspace", exact: true }),
    ],
    [
      "Auxiliary AI",
      page.getByRole("heading", { name: "Auxiliary AI", exact: true }),
    ],
  ]) {
    await page.getByRole("link", { name, exact: true }).click();
    await expect(body).toBeVisible();
    if (name !== "Projects view")
      await expect(page.locator(".resource-panel .project-row")).toHaveCount(0);
  }
});
test("resource bodies fit a mobile viewport", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await fixture(page);
  for (const name of ["Accounts", "Memory", "Search"]) {
    await page.getByRole("link", { name, exact: true }).click();
    await expect(page.locator("main")).toBeVisible();
    const bounds = await page.locator("main").boundingBox();
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(845);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(391);
  }
});
test("inline session mentions and secret chips preserve the prompt and storage boundary", async ({
  page,
}) => {
  const requests = await fixture(page, true);
  await page
    .getByRole("link", { name: /Research session oak/ })
    .first()
    .click();
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.pressSequentially("ask @el");
  await expect(
    page.getByRole("grid", { name: "Session mentions" }),
  ).toBeVisible();
  await input.press("Tab");
  await expect(input).toHaveValue("ask @elm ");
  await input.fill("");
  await input.pressSequentially("pass /@red");
  await input.press("Tab");
  await page
    .getByRole("dialog", { name: "Secret", exact: true })
    .locator("input[type=password]")
    .fill("synthetic-private-value");
  await page.getByRole("button", { name: "Insert secret chip" }).click();
  await expect(
    page.getByRole("button", { name: "Secret file reference", exact: true }),
  ).toBeVisible();
  await expectChipDisplay(input, "pass Secret");
  expect(await input.inputValue()).not.toContain("synthetic-private-value");
  expect(
    await page.evaluate(() =>
      JSON.stringify({ ...sessionStorage, ...localStorage }),
    ),
  ).not.toContain("synthetic-private-value");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(() =>
      requests.some(
        (r) =>
          r.path.endsWith("SessionService/Send") &&
          r.q.text?.includes("secret placed at /run/cxz-secrets/test"),
      ),
    )
    .toBe(true);
  expect(
    requests.filter((r) => r.path.endsWith("SessionService/Send")).at(-1).q
      .text,
  ).not.toContain("synthetic-private-value");
});
test("account registration, session creation and confirmed erasure use resource APIs", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Accounts", exact: true }).click();
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add account", exact: true });
  await add
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("personal");
  await add.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect
    .poll(() =>
      requests.some(
        (r) =>
          r.path.endsWith("AccountService/Add") && r.q.alias === "personal",
      ),
    )
    .toBe(true);
  await page.getByRole("link", { name: "Sessions view" }).click();
  const panel = page.getByRole("complementary", {
    name: "Session list",
    exact: true,
  });
  await expect(panel).not.toContainText("127.0.0.1");
  const body = panel.locator(".panel-scroll-region");
  const appearance = await panel.evaluate((el) => ({
    background: getComputedStyle(el).backgroundColor,
    body: getComputedStyle(el.querySelector(".panel-scroll-region"))
      .backgroundColor,
  }));
  expect(appearance.body).not.toBe(appearance.background);
  expect(
    await body.evaluate((el) => parseFloat(getComputedStyle(el).borderRadius)),
  ).toBeGreaterThan(0);
  const menuTrigger = panel.getByRole("button", {
    name: "Session panel options",
    exact: true,
  });
  await menuTrigger.hover();
  await menuTrigger.evaluate((el) =>
    Promise.all(el.getAnimations().map((animation) => animation.finished)),
  );
  const hoverBackground = await menuTrigger.evaluate(
    (el) => getComputedStyle(el).backgroundColor,
  );
  await expect(menuTrigger).toHaveCSS("border-right-width", "0px");
  await panel
    .getByRole("button", { name: "Session panel options", exact: true })
    .click();
  await page.mouse.move(0, 0);
  await expect(menuTrigger).toHaveCSS("background-color", hoverBackground);

  await page
    .getByRole("menu", { name: "Session panel options", exact: true })
    .evaluate(async (el) => {
      const shell = getComputedStyle(el);
      if (
        shell.filter !== "none" ||
        shell.maskImage !== "none" ||
        shell.transform !== "none"
      )
        throw new Error("Menu border must remain outside the reveal effects");
      const content = el.querySelector('[data-enter-effect="reveal"]');
      const animations = el.getAnimations({ subtree: true });
      if (!animations.length)
        throw new Error("Menu entrance animation missing");
      const entrance = content.getAnimations()[0];
      entrance.pause();
      entrance.currentTime = 0;
      const start = new DOMMatrix(getComputedStyle(content).transform);
      if (start.m41 >= 0 || start.m42 >= 0)
        throw new Error("Menu must reveal from the top left");
      if (getComputedStyle(content).filter !== "blur(4px)")
        throw new Error("Reveal must start blurred");
      if (getComputedStyle(content).maskImage === "none")
        throw new Error("Reveal must use a directional mask");
      entrance.currentTime = Number(entrance.effect.getTiming().duration) * 0.7;
      const settled = getComputedStyle(content);
      if (settled.filter !== "blur(0px)" || settled.maskPosition !== "0% 0%")
        throw new Error("Reveal edges must be clear before the animation ends");
      entrance.play();
      await Promise.all(animations.map((animation) => animation.finished));
    });
  const menuBox = await page
    .getByRole("menu", { name: "Session panel options", exact: true })
    .boundingBox();
  const triggerBox = await panel
    .getByRole("button", { name: "Session panel options", exact: true })
    .boundingBox();
  const panelBox = await panel.boundingBox();
  const edge = panelBox.x + panelBox.width;
  const border = await panel
    .locator("header")
    .evaluate((el) => parseFloat(getComputedStyle(el).borderRightWidth));
  expect(menuBox.y).toBeCloseTo(triggerBox.y, 0);
  expect(menuBox.x - edge).toBeCloseTo(
    edge - border - triggerBox.x - triggerBox.width,
    0,
  );
  await page
    .getByRole("menuitem", { name: "Add session", exact: true })
    .click();
  const create = page.getByRole("dialog", { name: "Add session", exact: true });
  await expect(create).toBeVisible();
  await create.evaluate((node) =>
    Promise.all(
      node
        .getAnimations({ subtree: true })
        .filter(
          (animation) =>
            animation.effect?.getComputedTiming().endTime !== Infinity,
        )
        .map((animation) => animation.finished),
    ),
  );
  const box = await create.boundingBox();
  const viewport = page.viewportSize();
  expect(box.x + box.width / 2).toBeCloseTo(viewport.width / 2, 0);
  expect(box.y + box.height / 2).toBeCloseTo(viewport.height / 2, 0);
  await expect
    .poll(() => create.evaluate((el) => el.contains(document.activeElement)))
    .toBe(true);
  await page.keyboard.press("Escape");
  await expect(create).toHaveCount(0);
  await expect(
    panel.getByRole("button", { name: "Session panel options", exact: true }),
  ).toBeFocused();
  await page.emulateMedia({ reducedMotion: "reduce" });
  await panel
    .getByRole("button", { name: "Session panel options", exact: true })
    .click();
  const reducedMenu = page.getByRole("menu", {
    name: "Session panel options",
    exact: true,
  });
  await expect(reducedMenu).toHaveCSS("animation-name", "none");
  await expect(reducedMenu).toHaveCSS("mask-image", "none");
  await expect(reducedMenu).toHaveCSS("filter", "none");
  await expect(reducedMenu.locator('[data-enter-effect="reveal"]')).toHaveCSS(
    "animation-name",
    "none",
  );

  await page
    .getByRole("menuitem", { name: "Add session", exact: true })
    .click();
  await page.getByRole("combobox", { name: "Account", exact: true }).click();
  await page.getByRole("option", { name: /Work account/ }).click();
  await page
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("New title");
  await create.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect
    .poll(() =>
      requests.some(
        (r) =>
          r.path.endsWith("SessionService/Add") &&
          r.q.name === "New title" &&
          r.q.authBinding.bindingId === "binding",
      ),
    )
    .toBe(true);
  await expect(create).toHaveCount(0);
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await page
    .locator("main")
    .getByRole("button", { name: "Controls for Research session", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Erase", exact: true }).click();
  const erase = page.getByRole("dialog", {
    name: "Erase session",
    exact: true,
  });
  await expect(erase).toContainText("This cannot be undone.");
  await erase.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(erase).toHaveCount(0);
  expect(requests.some((r) => r.path.endsWith("SessionService/Erase"))).toBe(
    false,
  );
  await page
    .locator("main")
    .getByRole("button", { name: "Controls for Research session", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Erase", exact: true }).click();
  await erase.getByRole("button", { name: "Erase", exact: true }).click();
  await expect
    .poll(() => requests.some((r) => r.path.endsWith("SessionService/Erase")))
    .toBe(true);
  await expect(erase).toHaveCount(0);
  await expect(page.locator("main tbody tr")).toHaveCount(0);
  expect(
    requests.some((r) => /SessionService\/(Restore|Purge)$/.test(r.path)),
  ).toBe(false);
});
test("Search has no resource panel and submits its filters only on search", async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date("2026-11-05T12:00:00Z"));
  const requests = await fixture(page, false, true);
  await page.getByRole("link", { name: "Search", exact: true }).click();
  await expect(page.locator(".resource-panel")).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Search", exact: true }),
  ).toHaveCount(0);
  const input = page.getByRole("searchbox", {
    name: "Search conversations",
    exact: true,
  });
  await input.fill("Earlier");
  await page
    .getByRole("combobox", { name: "Search project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Docs playground", exact: true })
    .click();
  await page.getByRole("button", { name: "Match case", exact: true }).click();
  await page
    .getByRole("button", { name: "Use regular expression", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Filters", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("switch", { name: "Include tools", exact: true }),
  ).toHaveCount(0);
  await pickSearchDate(page, "From (UTC)", "2026-11-01");
  await pickSearchDate(page, "Until (UTC, exclusive)", "2026-11-05");
  expect(
    requests.filter((r) => r.path === "/cxz.SessionService/Search"),
  ).toHaveLength(0);
  await input.press("Enter");
  await expect(
    page.getByRole("link", { name: /Earlier response/ }),
  ).toBeVisible();
  const request = requests.find(
    (r) => r.path === "/cxz.SessionService/Search",
  ).q;
  expect(request).toMatchObject({
    query: "Earlier",
    match: "regex",
    projects: [projectFixture(2).uuid],
    since: "2026-11-01T00:00:00Z",
    until: "2026-11-05T00:00:00Z",
  });
  expect(request.ignoreCase ?? false).toBe(false);
  expect(request.includeTools ?? false).toBe(false);
  await page
    .getByRole("button", { name: "Reset filters", exact: true })
    .click();
  await expect(
    page.getByRole("link", { name: /Earlier response/ }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect
    .poll(
      () =>
        requests.filter((r) => r.path === "/cxz.SessionService/Search").length,
    )
    .toBe(2);
  expect(
    requests.filter((r) => r.path === "/cxz.SessionService/Search").at(-1).q
      .projects ?? [],
  ).toEqual([]);
});
test("search opens the matched response and queue recall preserves its text", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Search", exact: true }).click();
  await page
    .getByRole("searchbox", { name: "Search conversations" })
    .fill("Earlier");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await page.getByRole("link", { name: /Earlier response/ }).click();
  await expect(page).toHaveURL(/[?]seq=2$/);
  await expect(
    page.getByRole("button", { name: "Recall", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Recall", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveValue("Queued review request");
  expect(
    requests.some(
      (r) => r.path.endsWith("SessionService/Send") && r.q.cancel === true,
    ),
  ).toBe(true);
});
test("conversation search uses loaded messages immediately without a Search request", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.locator('.tree-session[href="/c/_/session/oak"]').click();
  await expect(page.locator(".transcript")).toContainText("Current response");
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await input.fill("Earlier");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 2",
  );
  await search.getByRole("button", { name: /Earlier response/ }).click();
  await expect(input).toBeFocused();
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await expect(search).toBeVisible();
  await input.fill("query not in loaded messages");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 0 of 0",
  );
  await expect(search.getByText("No matches", { exact: true })).toBeVisible();
  await input.fill("Current");
  await input.fill("Earlier");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 2",
  );
  await expect(search).not.toContainText("Current response");
  expect(
    requests.filter((r) => r.path.endsWith("SessionService/Search")),
  ).toHaveLength(0);
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});

test("memory edits carry the revision and keep drafts on a conflict", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await page.getByRole("link", { name: /Research notes/ }).click();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const editor = page.locator(".monaco-editor textarea");
  await expect(editor).toBeVisible();
  await editor.focus();
  await editor.press("Control+a");
  await editor.fill("conflict");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expectAPIError(page, "Document changed");
  expect(
    requests.find((r) => r.path === "/cxz.MemoryService/Patch")?.q.baseRevision,
  ).toBe("1");
  await expect(
    page.getByText("Research notes · Unsaved", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Memory actions", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Reload", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Discard unsaved changes" }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Discard unsaved changes" }),
  ).toHaveCount(0);
  await expect(page.locator(".monaco-editor")).toContainText(
    "External version",
  );
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await page.locator(".monaco-editor .view-lines").click();
  await expect(editor).toBeFocused();
  await editor.press("Control+a");
  await editor.pressSequentially("Merged notes");
  await expect(
    page.getByText("Research notes · Unsaved", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect
    .poll(
      () =>
        requests.filter((r) => r.path === "/cxz.MemoryService/Patch").at(-1)?.q
          .baseRevision,
    )
    .toBe("2");
});
test("usage, background tasks and auxiliary controls are connected", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await page
    .getByRole("table", { name: "Projects", exact: true })
    .getByRole("link", { name: "Demo project project", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Recent 30 days", exact: true }),
  ).toBeVisible();
  await expect
    .poll(() =>
      requests.some((r) => r.path.endsWith("ConversationStats") && r.q.project),
    )
    .toBe(true);
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await page.getByRole("link", { name: "oak", exact: false }).first().click();
  await page
    .getByRole("button", {
      name: /active background jobs?|Background job status unavailable/,
    })
    .click();
  await expect(page.getByText("Build assets", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Session menu" }).click();
  await page
    .getByRole("menuitem", { name: "Auxiliary AI", exact: true })
    .click();
  await page.getByRole("button", { name: "Run now" }).first().click();
  await expect
    .poll(() => requests.some((r) => r.path.endsWith("AuxRun")))
    .toBe(true);
  await expect(page.getByText("running", { exact: true })).toBeVisible();
});

test("local session search Enter visits loaded responses before wrapping without search or target history requests", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.locator('.tree-session[href="/c/_/session/oak"]').click();
  await expect(page.locator(".transcript")).toContainText("Current response");
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await input.fill("response");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 2",
  );
  const historyCount = () =>
    requests.filter((r) => /SessionService\/(History|Transcript)$/.test(r.path))
      .length;
  const before = historyCount();
  const active = () =>
    page.evaluate(() => {
      const range = [
        ...(CSS.highlights.get("conversation-search-current") ?? []),
      ][0];
      return range?.startContainer.parentElement?.closest("[data-row]")?.dataset
        .row;
    });
  await input.press("Enter");
  await expect.poll(active).toBe("4");
  await expect(input).toBeFocused();
  await input.press("Enter");
  await expect.poll(active).toBe("2");
  await input.press("Enter");
  await expect.poll(active).toBe("4");
  expect(
    requests.filter((r) => r.path.endsWith("SessionService/Search")),
  ).toHaveLength(0);
  expect(historyCount()).toBe(before);
  await expect(input).toBeFocused();
});

test("project picker supports fuzzy search, keyboard selection and dark form backgrounds", async ({
  page,
}) => {
  await fixture(page, false, true);
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await page.getByRole("button", { name: "New session", exact: true }).click();
  const create = page
    .getByRole("dialog", { name: "Add session", exact: true })
    .or(page.getByRole("region", { name: "New session", exact: true }));
  await expect(create).toBeVisible();
  const projectPicker = create.getByRole("combobox", {
    name: "Project",
    exact: true,
  });
  const titleInput = create.getByRole("textbox", {
    name: "Title",
    exact: true,
  });
  // The anchored creation dialog moves during entrance. Measure the value
  // overlay against its settled trigger, rather than an intermediate frame.
  await create.evaluate((node) =>
    Promise.all(
      node
        .getAnimations({ subtree: true })
        .filter(
          (animation) =>
            animation.effect?.getComputedTiming().endTime !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => {})),
    ),
  );
  await page.mouse.move(0, 0);
  await expect(projectPicker).toHaveCSS(
    "background-color",
    await titleInput.evaluate((el) => getComputedStyle(el).backgroundColor),
  );
  const triggerBounds = await projectPicker
    .locator(".meta-value")
    .boundingBox();
  await projectPicker.click();
  const search = page.getByRole("combobox", {
    name: "Search Project",
    exact: true,
  });
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("Demo project");
  await search.press("Enter");
  await expect(search).toHaveCount(0);
  await expect(projectPicker).toHaveText("Demo project");
  await projectPicker.click();
  await expect(search).toHaveValue("Demo project");

  await expect
    .poll(async () => {
      const bounds = await search.boundingBox();
      return Math.max(
        Math.abs(bounds.x - triggerBounds.x),
        Math.abs(
          bounds.y +
            bounds.height / 2 -
            triggerBounds.y -
            triggerBounds.height / 2,
        ),
      );
    })
    .toBeLessThan(0.5);
  await expect(search).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(
    page.getByRole("option", { name: "Demo project", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toBeVisible();
  await search.fill("dcsplg");
  await expect(search).toBeFocused();
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toBeVisible();
  await search.fill("not-a-project");
  await expect(
    page
      .getByRole("listbox", { name: "Project choices", exact: true })
      .getByRole("status"),
  ).toHaveText("No matches");
  await search.press("Enter");
  await expect(search).toBeVisible();
  await search.fill("DOCS");
  await search.press("ArrowDown");
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(search).toHaveCount(0);
  await expect(create).toBeVisible();
  await expect(projectPicker).toBeFocused();
  await projectPicker.click();
  await expect(search).toHaveValue("Docs playground");
  await search.fill("");
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("option", { name: "Demo project", exact: true }),
  ).toBeVisible();
  await search.press("Escape");
  await expect(create).toBeVisible();
});

const memoryProjectPath = "/c/_/project/project/memory";
function memoryFixtureUUID(name) {
  const hex = Buffer.from(name.padEnd(16, "0")).toString("hex");
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20),
  ].join("-");
}
const researchPath = `${memoryProjectPath}/${memoryFixtureUUID("memory")}`;
const handoffPath = `${memoryProjectPath}/${memoryFixtureUUID("handoff")}`;
const memoryDocument = (page) =>
  page.getByRole("region", { name: "Memory document", exact: true });
test("Memory session filtering reuses cached project metadata from the first selection", async ({
  page,
}) => {
  const requests = await fixture(page, false, false, memoryProjectPath);
  await expect(
    page
      .getByRole("region", { name: "Project memories", exact: true })
      .getByRole("link"),
  ).toHaveCount(2);
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Research session", exact: true })
    .click();
  await expect(
    page
      .getByRole("region", { name: "Project memories", exact: true })
      .getByRole("link"),
  ).toHaveCount(1);
  await expect(
    page.getByRole("link", { name: "Research notes", exact: true }),
  ).toBeVisible();
  const request = requests
    .filter((r) => r.path === "/cxz.MemoryService/List")
    .at(-1).q;
  expect(request.project.id).toBe(projectFixture().id);
  expect(request.session).toBeUndefined();
  expect(
    requests.filter((r) => r.path === "/cxz.MemoryService/List"),
  ).toHaveLength(1);
  expect(request.scope).toBe("MEMORY_SCOPE_ACTIVE");
  expect(
    requests.filter((r) => r.path === "/cxz.MemoryService/Read"),
  ).toHaveLength(0);
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name: "All sessions", exact: true }).click();
  await expect(
    page
      .getByRole("region", { name: "Project memories", exact: true })
      .getByRole("link"),
  ).toHaveCount(2);
  expect(
    requests.filter((r) => r.path === "/cxz.MemoryService/List").at(-1).q
      .session,
  ).toBeUndefined();
  await page.getByRole("link", { name: "Research notes", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const name = page.getByRole("textbox", { name: "Memory name", exact: true });
  await name.fill("Unsaved memory name");
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Discard unsaved changes",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(name).toHaveValue("Unsaved memory name");
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(
    page.getByRole("link", { name: "Research notes", exact: true }),
  ).toBeVisible();
  expect(
    requests.filter((r) => r.path === "/cxz.MemoryService/List"),
  ).toHaveLength(1);
});
test("Memory cache expiry refreshes in the background without hiding filtered cards", async ({
  page,
}) => {
  const start = new Date("2026-11-05T13:00:00Z");
  await page.clock.setFixedTime(start);
  const requests = await fixture(page, false, false, memoryProjectPath);
  const memories = page.getByRole("region", {
    name: "Project memories",
    exact: true,
  });
  await expect(memories.getByRole("link")).toHaveCount(2);
  let release;
  const pending = new Promise((resolve) => {
    release = resolve;
  });
  await page.route("**/cxz.MemoryService/List", async (route) => {
    await pending;
    await route.fallback();
  });
  await page.clock.setFixedTime(new Date(start.getTime() + 60_001));
  const refreshing = page.waitForRequest("**/cxz.MemoryService/List");
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Research session", exact: true })
    .click();
  await refreshing;
  await expect(memories).toHaveAttribute("aria-busy", "true");
  await expect(
    memories.getByRole("link", { name: "Research notes", exact: true }),
  ).toBeVisible();
  await expect(
    memories.getByText("Loading memories…", { exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name: "All sessions", exact: true }).click();
  await expect(memories.getByRole("link")).toHaveCount(2);
  await expect(memories).toHaveAttribute("aria-busy", "true");
  release();
  await expect(memories).toHaveAttribute("aria-busy", "false");
  await expect(memories.getByRole("alert")).toHaveCount(0);
  expect(
    requests.filter((r) => r.path === "/cxz.MemoryService/List"),
  ).toHaveLength(2);
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Research session", exact: true })
    .click();
  await expect(memories.getByRole("link")).toHaveCount(1);
  expect(
    requests.filter((r) => r.path === "/cxz.MemoryService/List"),
  ).toHaveLength(2);
});
test("Memory refresh failures retain cached cards and allow a retry", async ({
  page,
}) => {
  const start = new Date("2026-11-05T13:00:00Z");
  await page.clock.setFixedTime(start);
  await fixture(page, false, false, memoryProjectPath);
  const memories = page.getByRole("region", {
    name: "Project memories",
    exact: true,
  });
  await expect(memories.getByRole("link")).toHaveCount(2);
  const fail = (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        code: "unavailable",
        message: "Refresh temporarily unavailable",
      }),
    });
  await page.route("**/cxz.MemoryService/List", fail);
  await page.clock.setFixedTime(new Date(start.getTime() + 60_001));
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Research session", exact: true })
    .click();
  await expectAPIError(page, "Refresh temporarily unavailable");
  await expect(
    memories.getByRole("link", { name: "Research notes", exact: true }),
  ).toBeVisible();
  await page.unroute("**/cxz.MemoryService/List", fail);
  await memories.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(memories.getByRole("alert")).toHaveCount(0);
  await expect(memories).toHaveAttribute("aria-busy", "false");
  await expect(
    memories.getByRole("link", { name: "Research notes", exact: true }),
  ).toBeVisible();
});
test("Memory collection, document links, reload and browser history agree", async ({
  page,
}) => {
  const requests = await fixture(page, false, false, memoryProjectPath);
  const research = page.getByRole("link", { name: /Research notes/ });
  const handoff = page.getByRole("link", { name: /Handoff notes/ });
  await expect(research).toHaveAttribute("href", researchPath);
  await expect(handoff).toHaveAttribute("href", handoffPath);
  await expect(memoryDocument(page)).toHaveCount(0);
  expect(
    requests.filter((r) => r.path === "/cxz.MemoryService/Read"),
  ).toHaveLength(0);
  await research.click();
  await expect(page).toHaveURL(new RegExp(`${researchPath}$`));
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Back to memories", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await handoff.click();
  await expect(page).toHaveURL(new RegExp(`${handoffPath}$`));
  await expect(
    page.getByRole("heading", { name: "Handoff", exact: true }),
  ).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(new RegExp(`${memoryProjectPath}$`));
  await expect(memoryDocument(page)).toHaveCount(0);
  await page.goBack();
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(new RegExp(`${memoryProjectPath}$`));
  await expect(memoryDocument(page)).toHaveCount(0);
  await page.goForward();
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`${memoryProjectPath}$`));
  await expect(memoryDocument(page)).toHaveCount(0);
});
test("Memory browser navigation confirms dirty drafts before changing the selected document", async ({
  page,
}) => {
  await fixture(page, false, false, researchPath);
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await page.getByRole("link", { name: /Handoff notes/ }).click();
  await expect(
    page.getByRole("heading", { name: "Handoff", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const name = page.getByRole("textbox", { name: "Memory name", exact: true });
  await name.fill("Uncommitted name");
  await page.goBack();
  const dialog = page.getByRole("dialog", {
    name: "Discard unsaved changes",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`${handoffPath}$`));
  await expect(name).toHaveValue("Uncommitted name");
  await page.goBack();
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`${memoryProjectPath}$`));
  await page.getByRole("link", { name: /Research notes/ }).click();
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
});
for (const [name, path, message] of [
  ["invalid ID", `${memoryProjectPath}/bad-id`, "Invalid memory ID."],
  [
    "missing document",
    `${memoryProjectPath}/${memoryFixtureUUID("missing")}`,
    "Memory not found",
  ],
  [
    "wrong project",
    `/c/_/project/${projectFixture(2).uuid}/memory/${memoryFixtureUUID("memory")}`,
    "Memory not found",
  ],
])
  test(`Memory deep links never substitute another document: ${name}`, async ({
    page,
  }) => {
    await fixture(page, false, true, path);
    if (name === "invalid ID")
      await expect(page.getByRole("main").getByRole("alert")).toContainText(
        message,
      );
    else await expectAPIError(page, message);
    await expect(
      page.getByRole("heading", { name: "Shared notes", exact: true }),
    ).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`${path}$`));
  });

for (const document of [false, true])
  test(`Memory UUID ${document ? "document" : "collection"} bookmarks stay valid while links prefer aliases`, async ({
    page,
  }) => {
    const path = `/c/_/project/${projectFixture().uuid}/memory${document ? `/${memoryFixtureUUID("memory")}` : ""}`;
    const requests = await fixture(page, false, false, path);
    const research = page.getByRole("link", { name: /Research notes/ });
    await expect(
      page.getByRole("link", { name: "Memory", exact: true }),
    ).toHaveAttribute("href", memoryProjectPath);
    await expect(page).toHaveURL(new RegExp(`${path}$`));
    if (document) {
      await expect(
        page.getByRole("heading", { name: "Shared notes", exact: true }),
      ).toBeVisible();
      expect(
        requests.find((r) => r.path === "/cxz.MemoryService/Read").q.ref.project
          .id,
      ).toBe(projectFixture().id);
      const back = page.getByRole("link", {
        name: "Memory",
        exact: true,
      });
      await expect(back).toHaveAttribute("href", memoryProjectPath);
      await back.click();
    } else await expect(memoryDocument(page)).toHaveCount(0);
    await expect(research).toHaveAttribute("href", researchPath);
    expect(
      requests.find((r) => r.path === "/cxz.MemoryService/List").q.project.id,
    ).toBe(projectFixture().id);
    await page.getByRole("link", { name: /Handoff notes/ }).click();
    await expect(page).toHaveURL(new RegExp(`${handoffPath}$`));
    await expect(
      page.getByRole("heading", { name: "Handoff", exact: true }),
    ).toBeVisible();
    await page.reload();
    await expect(
      page.getByRole("heading", { name: "Handoff", exact: true }),
    ).toBeVisible();
  });
test("Memory links fall back to project UUIDs without an alias", async ({
  page,
}) => {
  const path = `/c/_/project/${projectFixture().uuid}/memory`;
  await fixture(page, false, false, path, "");
  await expect(
    page.getByRole("link", { name: "Memory", exact: true }),
  ).toHaveAttribute("href", path);
  await expect(
    page.getByRole("link", { name: /Research notes/ }),
  ).toHaveAttribute("href", `${path}/${memoryFixtureUUID("memory")}`);
  await page.getByRole("link", { name: /Research notes/ }).click();
  await expect(
    page.getByRole("heading", { name: "Shared notes", exact: true }),
  ).toBeVisible();
});
test("Memory project picker navigates using the chosen project's alias", async ({
  page,
}) => {
  const requests = await fixture(page, false, true, memoryProjectPath);
  await expect(
    page.getByRole("link", { name: /Research notes/ }),
  ).toBeVisible();
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Docs playground", exact: true })
    .click();
  await expect(page).toHaveURL(/\/c\/_\/project\/docs-playground\/memory$/);
  await expect(
    page.getByRole("link", { name: "Memory", exact: true }),
  ).toHaveAttribute("href", "/c/_/project/docs-playground/memory");
  await expect
    .poll(
      () =>
        requests.filter((r) => r.path === "/cxz.MemoryService/List").at(-1)?.q
          .project.id,
    )
    .toBe(projectFixture(2).id);
  await expect(memoryDocument(page)).toHaveCount(0);
});

test("conversation menu Erase confirms, cancels and removes the session by stable ID", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page
    .getByRole("link", { name: /Research session oak/ })
    .first()
    .click();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  const openErase = async () => {
    await page
      .getByRole("button", { name: "Session menu", exact: true })
      .click();
    const menu = page.getByRole("menu", { name: "Session menu", exact: true });
    await expect(menu.getByRole("menuitem").last()).toHaveText("Erase");
    await menu.getByRole("menuitem", { name: "Erase", exact: true }).click();
  };
  await openErase();
  const dialog = page.getByRole("dialog", {
    name: "Erase session",
    exact: true,
  });
  await expect(dialog).toContainText("Research session");
  await expect(dialog).toContainText("This cannot be undone.");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(
    requests.filter((r) => r.path === "/cxz.SessionService/Erase"),
  ).toHaveLength(0);
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  await openErase();
  await dialog.getByRole("button", { name: "Erase", exact: true }).click();
  await expect(page).toHaveURL(/\/c\/_\/session$/);
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(0);
  expect(
    requests.filter((r) => r.path === "/cxz.SessionService/Erase"),
  ).toHaveLength(1);
  expect(
    requests.find((r) => r.path === "/cxz.SessionService/Erase").q,
  ).toEqual({ id: Buffer.from("fixture".padEnd(16, "0")).toString("base64") });
});
test("failed conversation menu Erase preserves the session and draft for retry", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page
    .getByRole("link", { name: /Research session oak/ })
    .first()
    .click();
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Keep this draft");
  const eraseEndpoint = "**/cxz.SessionService/Erase";
  await page.route(eraseEndpoint, (route) =>
    route.fulfill({
      status: 503,
      json: { code: "unavailable", message: "Erase unavailable" },
    }),
  );
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  await page.getByRole("menuitem", { name: "Erase", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Erase session",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Erase", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Action failed");
  await expect(input).toHaveValue("Keep this draft");
  await expect(page).toHaveURL(/\/c\/_\/session\/oak$/);
  expect(
    requests.filter((r) => r.path === "/cxz.SessionService/Erase"),
  ).toHaveLength(0);
  await page.unroute(eraseEndpoint);
  await dialog.getByRole("button", { name: "Erase", exact: true }).click();
  await expect(page).toHaveURL(/\/c\/_\/session$/);
  expect(
    requests.filter((r) => r.path === "/cxz.SessionService/Erase"),
  ).toHaveLength(1);
});

test("Errors page loads directly and shows all statuses after a dialog handoff and reload", async ({
  page,
}) => {
  await fixture(page, false, false, "/c/_/errors");
  const main = page.getByRole("main", { name: "Errors", exact: true });
  await expect(
    main.getByRole("table", { name: "Errors", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Errors", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await dialog.getByRole("radio", { name: /^History / }).click();
  const link = dialog.getByRole("link", {
    name: "Open errors page",
    exact: true,
  });
  await expect(link).toHaveAttribute("href", "/c/_/errors");
  await link.click();
  await expect(dialog).toHaveCount(0);
  await expect(page).toHaveURL(/\/c\/_\/errors$/);
  await expect(
    main.getByRole("table", { name: "Errors", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(main.getByRole("radio")).toHaveCount(0);
});
