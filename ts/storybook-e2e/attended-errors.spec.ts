import { expect, test } from "@playwright/test";

for (const mode of ["session", "history", "lookup", "missing-transcript"])
  for (const width of [1200, 390])
    test(`${mode} read failure stays inline and recorded without a sidebar alert at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 850 });
      await page.goto(
        `/iframe.html?id=workspace-errors--attended-${mode}&viewMode=story`,
      );
      const message =
        mode === "history"
          ? "Conversation history is unavailable"
          : "session not found";
      await expect(
        page.getByRole("alert").filter({ hasText: message }),
      ).toBeVisible();
      const trigger = page.getByRole("button", { name: "Errors", exact: true });
      await expect(trigger).not.toHaveAttribute("data-active");
      await expect(trigger.getByRole("status")).toHaveText("0 error alerts");
      await trigger.click();
      const panel = page.getByRole("dialog", { name: "Errors", exact: true });
      await expect(panel.getByRole("table")).toContainText(message);
      await panel
        .getByRole("link", { name: "Open errors page", exact: true })
        .click();
      const main = page.getByRole("main", { name: "Errors", exact: true });
      await expect(main).toContainText(message);
      await main.getByRole("row").filter({ hasText: message }).first().click();
      await expect(
        page.getByRole("dialog", { name: "Error details", exact: true }),
      ).toContainText("Excluded from alerts");
    });

test("a failed foreground page read after loading stays inline without alerting", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors--attended-pagination&viewMode=story",
  );
  await expect(
    page.getByText("Recovered conversation", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Read older messages", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "session not found: missing-runtime",
  );
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  const panel = page.getByRole("dialog", { name: "Errors", exact: true });
  await panel
    .getByRole("link", { name: "Open errors page", exact: true })
    .click();
  await page
    .getByRole("main", { name: "Errors", exact: true })
    .getByRole("row")
    .filter({ hasText: "session not found: missing-runtime" })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Error details", exact: true }),
  ).toContainText("Excluded from alerts");
});

test("repeated account quota and project activity reads retain diagnostics without alerts", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors--optional-session-metadata&viewMode=story",
  );
  await expect(
    page.getByRole("img", { name: "Usage unavailable", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Refresh metadata", exact: true })
    .click();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const saved = JSON.parse(
          localStorage.getItem("cxz.attended-error-preview.v1:metadata") ??
            "null",
        );
        return saved?.entries[0]?.count ?? 0;
      }),
    )
    .toBeGreaterThanOrEqual(4);
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).not.toHaveAttribute("data-active");
  await expect(trigger.getByRole("status")).toHaveText("0 error alerts");
  await trigger.click();
  await expect(
    page
      .getByRole("dialog", { name: "Errors", exact: true })
      .getByRole("table"),
  ).toContainText("session not found: missing-runtime");
});

test("an unrelated unattended failure still alerts alongside an attended failure", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors--attended-session&viewMode=story",
  );
  await expect(page.getByRole("alert")).toContainText("session not found");
  await page
    .getByRole("button", { name: "Fail background request", exact: true })
    .click();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).toHaveAttribute("data-active", "true");
  await expect(trigger.getByRole("status")).toHaveText("1 error alerts");
  await trigger.click();
  const table = page
    .getByRole("dialog", { name: "Errors", exact: true })
    .getByRole("table");
  await expect(table).toContainText("session not found");
  await expect(table).toContainText("Account service is offline");
});

test("callers can suppress a generic RPC with notify:false and still inspect its record", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors--attended-session&viewMode=story",
  );
  await expect(page.getByRole("alert")).toContainText("session not found");
  await page
    .getByRole("button", { name: "Fail silent request", exact: true })
    .click();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  await expect(
    page
      .getByRole("dialog", { name: "Errors", exact: true })
      .getByRole("table"),
  ).toContainText("Account service is offline");
});

test("recovering the read resolves the retained attended episode without an alert", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors--attended-session&viewMode=story",
  );
  await expect(page.getByRole("alert")).toContainText("session not found");
  await page
    .getByRole("button", { name: "Recover conversation", exact: true })
    .click();
  await expect(
    page.getByText("Recovered conversation", { exact: true }),
  ).toBeVisible();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  const panel = page.getByRole("dialog", { name: "Errors", exact: true });
  await panel.getByRole("radio", { name: "History 1", exact: true }).click();
  await expect(
    panel.getByRole("row").filter({ hasText: "session not found" }),
  ).toHaveAttribute("title", /^Resolved/);
});

test("retries of the same attended load stay silent after leaving the view", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors--attended-history&viewMode=story",
  );
  await expect(page.getByRole("alert")).toContainText(
    "Conversation history is unavailable",
  );
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).not.toHaveAttribute("data-active");
  await page
    .getByRole("button", { name: "Hide conversation", exact: true })
    .click();
  await page.waitForTimeout(2500);
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  await expect(
    page
      .getByRole("dialog", { name: "Errors", exact: true })
      .getByRole("table"),
  ).toContainText("Conversation history is unavailable");
});
