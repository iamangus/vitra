import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const theme of ["light", "dark"]) {
  test(`phone ${theme} secondary views and graph controls`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.addInitScript((theme) => {
      localStorage.setItem("theme", theme);
      localStorage.setItem("noteMode", "view");
    }, theme);
    await page.route("**/api/activity?*", (route) =>
      route.fulfill({
        json: [
          {
            date: "2026-10-19",
            kind: "Decision",
            source: "Design/log",
            source_path: "Design",
            text: "Keep the document at the center of the workspace.",
          },
          {
            date: "2026-10-19",
            kind: "Update",
            source: "Design/log",
            source_path: "Design",
            text: "Connected the research notes with the implementation plan.",
          },
          {
            date: "2026-10-18",
            kind: "Discovery",
            source: "log",
            source_path: ".",
            text: "Small observations become useful knowledge when we connect them.",
          },
        ],
      }),
    );
    await page.goto("/activity");
    await expect(
      page.getByText("Keep the document at the center of the workspace."),
    ).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
    await expect(page).toHaveScreenshot(`activity-phone-${theme}.png`, {
      animations: "disabled",
      maxDiffPixelRatio: 0.005,
    });
    await page.route("**/api/search/semantic?*", (route) =>
      route.fulfill({
        json: [
          {
            path: "B",
            title: "Connected thinking",
            heading: "Research > Principles",
            type: "Reference",
            distance: 0.21,
            chunk:
              "A calm workspace gives your ideas room to breathe. Start with the document, then follow the connections that matter.",
          },
        ],
      }),
    );
    await page.goto("/search?q=workspace");
    await page.getByRole("button", { name: "Semantic", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "Connected thinking" }),
    ).toBeVisible();
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
    await expect(page).toHaveScreenshot(`semantic-phone-${theme}.png`, {
      animations: "disabled",
      maxDiffPixelRatio: 0.005,
    });
    await page.route("**/api/backlinks/B", (route) =>
      route.fulfill({
        json: [
          { path: "A", title: "A place for connected thinking" },
          {
            path: "Design/Long reference",
            title:
              "An exceptionally long reference title that should stay readable on a phone",
          },
        ],
      }),
    );
    await page.goto("/note/B");
    await expect(
      page.getByRole("region", { name: "Linked from" }),
    ).toContainText("A place for connected thinking");
    await expect(page).toHaveScreenshot(`backlinks-phone-${theme}.png`, {
      animations: "disabled",
      maxDiffPixelRatio: 0.005,
    });
    await page.route("**/api/graph", (route) =>
      route.fulfill({
        json: {
          nodes: [
            { id: "B", title: "Connected thinking", type: "Note" },
            { id: "A", title: "Daily practice", type: "Reference" },
          ],
          links: [{ source: "B", target: "A" }],
        },
      }),
    );
    await page.goto("/graph");
    await expect(page.locator(".graph-info")).toHaveText("2 notes · 1 link");
    await page
      .getByRole("textbox", { name: "Find a graph note" })
      .fill("Connected");
    await page.getByRole("button", { name: "Connected thinking Note" }).click();
    await expect(
      page.getByRole("button", { name: "Open note", exact: true }),
    ).toBeVisible();
    await page.mouse.move(0, 0);
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
    await expect(page).toHaveScreenshot(`graph-phone-${theme}.png`, {
      style: ".graph-canvas canvas { visibility: hidden !important; }",
      animations: "disabled",
      maxDiffPixelRatio: 0.005,
    });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
  });
}
