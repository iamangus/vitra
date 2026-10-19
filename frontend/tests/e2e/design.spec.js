import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const path = "Design/Research/Systems/Calm workspace";
const title = "Building a calmer knowledge workspace";
const content = `---
type: Reference
title: ${title}
tags: [design, knowledge]
---

# ${title}

A useful workspace gives your ideas room to breathe. Start with the document, then follow the connections that matter.

## A place for connected thinking

Capture the small observations that become bigger ideas. Link to [[B]] when a familiar thought needs more context.

> Keep the writing simple. Let the connections do the work.

### Principles to come back to

- Put the document first.
- Make every action clear and reversible where possible.
- Keep your knowledge portable in Markdown.

## Implementation notes

CODE_SAMPLE

<table><thead><tr><th>Component</th><th>Responsibility</th><th>State</th><th>Accessibility</th><th>Review</th><th>Notes</th></tr></thead><tbody><tr><td>Document workspace</td><td>Reading, editing and revision protection</td><td>Ready for review</td><td>Keyboard navigation</td><td>Required</td><td>A deliberately wide table cell to exercise overflow</td></tr></tbody></table>

## Next connections

Revisit this note when another idea needs a home.
`.replace(
  "CODE_SAMPLE",
  '```javascript\nconst workspace = { documentFirst: true, portable: "Markdown", connections: ["notes", "ideas", "references", "decisions"] };\n```',
);

async function fixture(request) {
  const existing = await request.get("/api/note/" + encodeURIComponent(path));
  if (existing.status() === 404)
    await request.post("/api/notes", { data: { path } });
  const note = await (
    await request.get("/api/note/" + encodeURIComponent(path))
  ).json();
  expect(
    (
      await request.post("/api/note/" + encodeURIComponent(path), {
        headers: { "If-Match": note.revision },
        data: content,
      })
    ).ok(),
  ).toBeTruthy();
}

async function boot(page, theme = "light") {
  await page.addInitScript((theme) => {
    localStorage.setItem("theme", theme);
    localStorage.setItem("noteMode", "view");
  }, theme);
}

for (const [size, width, height] of [
  ["desktop", 1440, 1000],
  ["tablet", 820, 1100],
  ["phone", 390, 844],
]) {
  for (const theme of ["light", "dark"]) {
    test(`${size} ${theme} reader visual and accessibility`, async ({
      page,
      request,
    }) => {
      await fixture(request);
      await page.setViewportSize({ width, height });
      await boot(page, theme);
      await page.goto("/note/" + encodeURIComponent(path));
      await expect(
        page.getByRole("heading", { name: title, exact: true }),
      ).toBeVisible();
      await expect(page.locator(".document-heading .eyebrow")).toHaveText(
        "Reference",
      );
      await expect(
        page.locator(".tag-chip").filter({ hasText: "# design" }),
      ).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBeTruthy();
      const axe = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze();
      expect(axe.violations).toEqual([]);
      await expect(page).toHaveScreenshot(`reader-${size}-${theme}.png`, {
        animations: "disabled",
        maxDiffPixelRatio: 0.005,
      });
      await expect(page.locator(".table-scroll")).toHaveCount(1);
      await page.locator(".table-scroll").scrollIntoViewIfNeeded();
      await expect(page).toHaveScreenshot(
        `reader-overflow-${size}-${theme}.png`,
        { animations: "disabled", maxDiffPixelRatio: 0.005 },
      );
    });
  }
}

test("home, search, activity, empty, loading and failure visuals", async ({
  page,
  request,
}) => {
  await fixture(request);
  await boot(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  for (const [name, url, ready] of [
    ["home", "/", "A little space to think clearly."],
    ["search", "/search?q=connections", title],
    ["activity", "/activity", "Activity"],
    ["missing", "/note/DesignMissingNote", "This page is waiting for an idea"],
  ]) {
    await page.goto(url);
    await expect(
      page.getByRole("heading", { name: ready, exact: true }),
    ).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await expect(page).toHaveScreenshot(`${name}-desktop.png`, {
      animations: "disabled",
      maxDiffPixelRatio: 0.005,
    });
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
  }
  await page.route("**/api/note/Delayed", () => new Promise(() => {}));
  await page.goto("/note/Delayed");
  await expect(page.getByLabel("Loading note")).toBeVisible();
  await expect(page).toHaveScreenshot("loading-desktop.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  await page.route("**/api/note/Failed", (route) =>
    route.fulfill({ status: 503, body: "offline" }),
  );
  await page.goto("/note/Failed");
  await expect(
    page.getByRole("heading", { name: "Unable to open this note" }),
  ).toBeVisible();
  await expect(page).toHaveScreenshot("failure-desktop.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
});

test("folder breadcrumbs reveal folders without navigating to nonexistent notes", async ({
  page,
  request,
}) => {
  await fixture(request);
  await boot(page);
  await page.goto("/note/" + encodeURIComponent(path));
  await page.getByRole("button", { name: "Systems", exact: true }).click();
  await expect(
    page.locator('summary[data-path="Design/Research/Systems"]'),
  ).toBeFocused();
  await expect(page.locator('a[data-path="' + path + '"]')).toBeVisible();
  await expect(page).toHaveURL(/Calm%20workspace/);
});

test("keyboard file dialogs, focus restore, touch move and suffix-safe rename", async ({
  page,
  request,
}) => {
  await boot(page);
  await page.goto("/");
  const trigger = page
    .getByRole("complementary", { name: "Vault navigation" })
    .getByRole("button", { name: "New note", exact: true });
  await trigger.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByLabel("Name", { exact: true }).fill("Keyboard.md");
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await trigger.click();
  await page.getByLabel("Name", { exact: true }).fill("Keyboard.md");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Note markdown" }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: "Note markdown" })
    .fill(
      "---\ntype: Note\ntitle: Keyboard.md\n---\n\nRaw source remains intact.",
    );
  await expect(page.getByRole("status")).toHaveText("Saved");
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Actions for Keyboard.md", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Move to…" }).click();
  await page.getByLabel("Destination folder").selectOption("Design");
  await page.getByRole("button", { name: "Move", exact: true }).click();
  await expect(
    page.locator('[data-note-path="Design/Keyboard.md"]'),
  ).toBeVisible();
  expect(
    (await (await request.get("/api/note/Design%2FKeyboard.md")).json())
      .content,
  ).toContain("Raw source remains intact.");
});

test("conflict recovery requires explicit discard and survives reload", async ({
  page,
  request,
}) => {
  await fixture(request);
  await boot(page);
  await page.route("**/api/note/" + encodeURIComponent(path), async (route) => {
    if (route.request().method() === "POST")
      return route.fulfill({ status: 409, body: "stale revision" });
    await route.continue();
  });
  await page.goto("/note/" + encodeURIComponent(path));
  await page.getByTitle("Edit mode", { exact: true }).click();
  await page
    .getByRole("textbox", { name: "Note markdown" })
    .fill("Retained local draft");
  await expect(page.getByRole("status")).toHaveText(
    "Conflict — draft retained",
  );
  await expect(page).toHaveScreenshot("conflict-desktop.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  await page.getByRole("button", { name: "Discard draft and reload" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(
    (
      await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze()
    ).violations,
  ).toEqual([]);
  await page.getByRole("button", { name: "Keep draft" }).click();
  await page.reload();
  await page.getByTitle("Edit mode", { exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Note markdown" }),
  ).toHaveValue("Retained local draft");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page).toHaveScreenshot("recovered-phone.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  await page.getByRole("button", { name: "Save recovered draft" }).click();
  await expect(page.getByRole("status")).toHaveText(
    "Conflict — draft retained",
  );
  await expect(page).toHaveScreenshot("conflict-phone.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
});

test("graph selection is keyboard accessible and opening is explicit", async ({
  page,
}) => {
  await boot(page, "dark");
  await page.route("**/api/graph", (route) =>
    route.fulfill({
      json: {
        nodes: [
          { id: "B", title: "B", type: "Note" },
          { id: "Research", title: "Connected thinking", type: "Reference" },
          { id: "Practice", title: "Daily practice", type: "Runbook" },
        ],
        links: [
          { source: "B", target: "Research" },
          { source: "Practice", target: "B" },
        ],
      },
    }),
  );
  await page.goto("/graph");
  await expect(page.locator(".graph-info")).toHaveText("3 notes · 2 links");
  await page.getByRole("textbox", { name: "Find a graph note" }).fill("B");
  await page.getByRole("button", { name: "B Note" }).focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("complementary", { name: "Selected note" }),
  ).toBeVisible();
  await expect(page.locator(".graph-canvas canvas")).toBeVisible();
  await expect(page.locator(".connection-count")).toHaveText("2 connections");
  await expect(page).toHaveURL("/graph");
  await page.mouse.move(0, 0);
  expect(
    (
      await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze()
    ).violations,
  ).toEqual([]);
  await expect(page).toHaveScreenshot("graph-selected-dark.png", {
    style: ".graph-canvas canvas { visibility: hidden !important; }",
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  await test.info().attach("Rendered graph (unmasked)", {
    body: await page.screenshot({
      path: test.info().outputPath("graph-rendered.png"),
    }),
    contentType: "image/png",
  });
  await page.getByRole("button", { name: "Open note", exact: true }).click();
  await expect(page.locator('[data-note-path="B"]')).toBeVisible();
});

test("split editor, outline, long title, contextual snippets and deferred graph loading", async ({
  page,
  request,
}) => {
  await fixture(request);
  await boot(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  const graphRequests = [];
  page.on("request", (request) => {
    if (/GraphView.*\.(js|css)/.test(request.url()))
      graphRequests.push(request.url());
  });
  await page.goto("/note/" + encodeURIComponent(path));
  await page.getByRole("button", { name: "Outline", exact: true }).click();
  await expect(
    page.getByRole("complementary", { name: "On this page" }),
  ).toBeVisible();
  await expect(page).toHaveScreenshot("outline-desktop.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  await page.getByRole("button", { name: "Close outline" }).click();
  await page.getByTitle("Split mode", { exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Note markdown" }),
  ).toHaveValue(content);
  await expect(page).toHaveScreenshot("split-desktop.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  await page.setViewportSize({ width: 820, height: 1100 });
  await expect(
    page.getByRole("textbox", { name: "Note markdown" }),
  ).toBeVisible();
  await expect(page.locator(".preview-pane")).toHaveCount(0);
  expect(graphRequests).toEqual([]);
  await page.keyboard.press("Control+k");
  await page.getByRole("textbox", { name: "Search notes" }).fill("connections");
  await page
    .getByRole("button", { name: "Search", exact: true })
    .last()
    .click();
  await expect(page.locator(".search-results mark").first()).toHaveText(
    /connections/i,
  );
  const matches = await (await request.get("/api/search?q=connections")).json();
  expect(matches.find((match) => match.path === path).snippet).toContain(
    "connections",
  );
  await page.route("**/api/note/LongTitle", (route) =>
    route.fulfill({
      json: {
        title:
          "An exceptionally long document title to verify readable wrapping and stable controls in a narrow knowledge workspace",
        content: "# Long title",
        html: "<p>A long title must not push actions out of reach.</p>",
        frontmatter: { type: "Note" },
        revision: "fixture",
      },
    }),
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/note/LongTitle");
  await page.getByTitle("View mode", { exact: true }).click();
  await expect(page).toHaveScreenshot("long-title-phone.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
});

test.describe("touch workspace", () => {
  test.use({
    hasTouch: true,
    isMobile: true,
    viewport: { width: 390, height: 844 },
  });
  test("navigation drawer, file actions, dialogs and outline work with touch", async ({
    page,
    request,
  }) => {
    await fixture(request);
    await boot(page, "dark");
    await page.goto("/note/" + encodeURIComponent(path));
    await page
      .getByRole("button", { name: "Open navigation", exact: true })
      .tap();
    await expect(page).toHaveScreenshot("navigation-phone-dark.png", {
      animations: "disabled",
      maxDiffPixelRatio: 0.005,
    });
    await page
      .getByRole("button", { name: "Actions for Calm workspace", exact: true })
      .tap();
    await page.getByRole("menuitem", { name: "Rename", exact: true }).tap();
    await expect(page.getByRole("dialog")).toBeVisible();
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
    await expect(page).toHaveScreenshot("rename-phone-dark.png", {
      animations: "disabled",
      maxDiffPixelRatio: 0.005,
    });
    await page.getByRole("button", { name: "Cancel", exact: true }).tap();
    await page
      .getByRole("complementary", { name: "Vault navigation" })
      .getByRole("button", { name: "Close navigation" })
      .tap();
    await page.getByRole("button", { name: "Outline", exact: true }).tap();
    await page
      .getByRole("button", { name: "Implementation notes", exact: true })
      .tap();
    await expect(
      page.getByRole("heading", { name: "Implementation notes", exact: true }),
    ).toBeInViewport();
    await page
      .locator(".table-scroll")
      .evaluate((node) => (node.scrollLeft = 400));
    expect(
      await page.locator(".table-scroll").evaluate((node) => node.scrollLeft),
    ).toBeGreaterThan(0);
  });
});

test("empty vault and search errors are actionable", async ({ page }) => {
  await boot(page);
  await page.route("**/api/files?*", (route) => route.fulfill({ json: [] }));
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Make room for your first idea" }),
  ).toBeVisible();
  await expect(page).toHaveScreenshot("empty-vault-desktop.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
  await page.route("**/api/search/semantic?*", (route) =>
    route.fulfill({ status: 503, body: "Provider unavailable" }),
  );
  await page.goto("/search?q=idea");
  await page.getByRole("button", { name: "Semantic", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Use full-text" }),
  ).toBeVisible();
  await expect(page).toHaveScreenshot("search-error-desktop.png", {
    animations: "disabled",
    maxDiffPixelRatio: 0.005,
  });
});
