import { test, expect } from "@playwright/test";
import { BinaryWriter, WireType } from "@bufbuild/protobuf/wire";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

test.use({
  viewport: { width: 1280, height: 900 },
  isMobile: false,
  hasTouch: false,
});
function stream(rows) {
  return Buffer.concat([
    ...rows.map((row) => {
      const body = Buffer.from(JSON.stringify(row));
      const header = Buffer.alloc(5);
      header.writeUInt32BE(body.length, 1);
      return Buffer.concat([header, body]);
    }),
    Buffer.from([2, 0, 0, 0, 2, 123, 125]),
  ]);
}
function loginDetail() {
  const w = new BinaryWriter()
    .tag(1, WireType.LengthDelimited)
    .string("PROJECT_LOGIN_REQUIRED")
    .tag(2, WireType.LengthDelimited)
    .string("cxz.auth");
  for (const [key, value] of [
    ["account", "work1"],
    ["session_key", "original-creation"],
  ])
    w.tag(3, WireType.LengthDelimited).bytes(
      new BinaryWriter()
        .tag(1, WireType.LengthDelimited)
        .string(key)
        .tag(2, WireType.LengthDelimited)
        .string(value)
        .finish(),
    );
  return {
    type: "google.rpc.ErrorInfo",
    value: Buffer.from(w.finish()).toString("base64"),
  };
}
async function fixture(page, { state = "idle", authFailure = true } = {}) {
  const id = (v) => Buffer.from(v.padEnd(16, "0")).toString("base64");
  const project = {
    id: id("project"),
    runtimeId: "project",
    alias: "project",
    name: "Demo project",
    workspace: "/workspace",
    listed: true,
    status: { state: "running" },
  };
  const account = {
    id: id("account"),
    alias: "work1",
    name: "Claude account",
    agent: "claude",
    authBackend: "project-local-oauth",
  };
  let run = "original-run";
  let authenticated = false;
  const requests = [];
  const sockets = [];
  const session = () => ({
    id: id("session"),
    runtimeId: "fixture",
    alias: "seal",
    name: "Claude session",
    clientId: "original-creation",
    listed: true,
    agent: "claude",
    project,
    account,
    status: { state, runId: run, lastSeq: "3", pending: [] },
  });
  const events = [
    {
      seq: "1",
      kind: "input",
      text: "Please continue",
      runId: run,
      timeMs: "1710000000000",
    },
    {
      seq: "2",
      kind: "assistant",
      text: "Failed to authenticate: OAuth session expired and could not be refreshed",
      runId: run,
      timeMs: "1710000001000",
    },
  ];
  const metadata = [
    {
      seq: "3",
      kind: "turn_end",
      runId: run,
      payload: Buffer.from(
        JSON.stringify({ authentication_required: authFailure }),
      ).toString("base64"),
    },
  ];
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
    await route.fulfill({
      body: readFileSync(file),
      contentType: file.endsWith(".js")
        ? "application/javascript"
        : file.endsWith(".css")
          ? "text/css"
          : file.endsWith(".html")
            ? "text/html"
            : "application/octet-stream",
    });
  });
  await page.route("**/cxz.*/*", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const raw = route.request().postDataBuffer();
    const q = raw?.length
      ? JSON.parse((raw[0] === 0 ? raw.subarray(5) : raw).toString())
      : {};
    requests.push({ path, q });
    const send = (json) => route.fulfill({ json });
    const frames = (rows) =>
      route.fulfill({
        contentType: "application/connect+json",
        body: stream(rows),
      });
    for (const [service, value] of [
      ["Project", project],
      ["Account", account],
      ["Session", session()],
    ]) {
      if (path === `/cxz.${service}Service/Get`) return send(value);
      if (path === `/cxz.${service}Service/List`)
        return send({ items: [value] });
      if (path === `/cxz.${service}Service/Watch`)
        return frames([{ items: [{ id: value.id, value }] }]);
    }
    if (path === "/cxz.SessionService/Transcript")
      return send({ events, metadata, snapshotSeq: "3" });
    if (path === "/cxz.SessionService/History") return send({ events: [] });
    if (
      [
        "/cxz.SessionService/Events",
        "/cxz.SessionService/Rows",
        "/cxz.SessionService/AuxEvents",
      ].includes(path)
    )
      return frames([]);
    if (path === "/cxz.SessionService/Stop") {
      state = "stopped";
      return send(session());
    }
    if (path === "/cxz.SessionService/Resume") {
      if (!authenticated)
        return route.fulfill({
          status: 400,
          json: {
            code: "failed_precondition",
            message: "account work1 needs Claude subscription login",
            details: [loginDetail()],
          },
        });
      run = "resumed-run";
      state = "idle";
      return send(session());
    }
    return send({});
  });
  await page.routeWebSocket("**/login/session?**", (socket) => {
    const entry = { socket, url: new URL(socket.url()), messages: [] };
    sockets.push(entry);
    socket.send(
      JSON.stringify({
        output: "Open https://claude.ai/oauth/authorize to log in.\n",
      }),
    );
    socket.onMessage((message) => entry.messages.push(message));
  });
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/");
  await page
    .getByRole("link", { name: /Claude session seal/ })
    .first()
    .click();
  return {
    requests,
    sockets,
    complete() {
      authenticated = true;
      sockets.at(-1).socket.send(JSON.stringify({ done: true }));
    },
    replaceRun() {
      run = "other-run";
    },
  };
}

test("expired Claude OAuth offers session login and resumes without replaying the prompt", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  expect(f.requests.filter((r) => r.path.endsWith("/Stop"))).toHaveLength(1);
  const target = f.sockets[0].url;
  expect(Object.fromEntries(target.searchParams)).toEqual({
    account: "work1",
    project: "project",
    key: "original-creation",
  });
  await panel.getByLabel("Authentication code").fill("test-code");
  await panel.getByRole("button", { name: "Submit code" }).click();
  await expect.poll(() => f.sockets[0].messages).toEqual(["test-code\n"]);
  await expect(panel.getByLabel("Authentication code")).toHaveValue("");
  f.complete();
  await expect(panel).toBeHidden();
  await expect
    .poll(() => f.requests.filter((r) => r.path.endsWith("/Resume")).length)
    .toBe(1);
  expect(f.requests.find((r) => r.path.endsWith("/Resume")).q.runId).toBe(
    "original-run",
  );
  expect(f.requests.some((r) => r.path.endsWith("/Send"))).toBe(false);
});

test("Resume's structured credential failure opens Claude login automatically", async ({
  page,
}) => {
  const f = await fixture(page, { state: "stopped", authFailure: false });
  await expect(
    page.getByRole("button", { name: "Log in again", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Session menu" }).click();
  await page.getByRole("menuitem", { name: "Resume", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  expect(f.requests.filter((r) => r.path.endsWith("/Stop"))).toHaveLength(0);
  f.complete();
  await expect
    .poll(() => f.requests.filter((r) => r.path.endsWith("/Resume")).length)
    .toBe(2);
  await expect(panel).toBeHidden();
});

test("canceling or failing OAuth never resumes; login can be retried", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  f.sockets[0].socket.send(
    JSON.stringify({ error: "Provider login canceled" }),
  );
  await expect(panel.getByRole("alert")).toHaveText("Provider login canceled");
  await panel.getByRole("button", { name: "Retry login" }).click();
  await expect.poll(() => f.sockets.length).toBe(2);
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(panel).toBeHidden();
  expect(
    f.requests.some(
      (r) => r.path.endsWith("/Resume") || r.path.endsWith("/Send"),
    ),
  ).toBe(false);
});

test("a run replaced during OAuth is not resumed", async ({ page }) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Provider login" }),
  ).toBeVisible();
  f.replaceRun();
  f.complete();
  await expect(
    page.getByRole("alert").filter({ hasText: "Session run changed" }),
  ).toBeVisible();
  expect(
    f.requests.some(
      (r) => r.path.endsWith("/Resume") || r.path.endsWith("/Send"),
    ),
  ).toBe(false);
});

test("Restart hands off its confirmation dialog to Claude login", async ({
  page,
}) => {
  const f = await fixture(page, { authFailure: false });
  await page.getByRole("button", { name: "Session menu" }).click();
  await page.getByRole("menuitem", { name: "Restart", exact: true }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Restart session",
    exact: true,
  });
  await confirmation
    .getByRole("button", { name: "Restart", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Provider login" }),
  ).toBeVisible();
  await expect(confirmation).toBeHidden();
  await expect.poll(() => f.sockets.length).toBe(1);
  f.complete();
  await expect
    .poll(() => f.requests.filter((r) => r.path.endsWith("/Resume")).length)
    .toBe(2);
  expect(f.requests.filter((r) => r.path.endsWith("/Stop"))).toHaveLength(1);
});

test("Escape cancels login before the card's closing animation completes", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  await expect.poll(() => f.sockets.length).toBe(1);
  await page.keyboard.press("Escape");
  f.complete();
  await expect(panel).toBeHidden();
  expect(
    f.requests.some(
      (r) => r.path.endsWith("/Resume") || r.path.endsWith("/Send"),
    ),
  ).toBe(false);
});
