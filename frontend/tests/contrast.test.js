import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const css = readFileSync(new URL("../src/app.css", import.meta.url), "utf8");
function luminance(hex) {
  const channels = hex
    .match(/[a-f\d]{2}/gi)
    .map((channel) => parseInt(channel, 16) / 255)
    .map((value) =>
      value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4,
    );
  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
}
function ratio(a, b) {
  const values = [luminance(a), luminance(b)].sort((a, b) => b - a);
  return (values[0] + 0.05) / (values[1] + 0.05);
}
for (const [name, selector] of [
  ["light", ":root"],
  ["dark", "html.dark"],
]) {
  test(`${name} normal-text design tokens meet WCAG AA contrast`, () => {
    const block = css.slice(css.indexOf(selector)).match(/\{([^}]+)\}/s)[1];
    const tokens = Object.fromEntries(
      [...block.matchAll(/--([\w-]+):\s*(#[a-f\d]{6});/gi)].map((match) => [
        match[1],
        match[2],
      ]),
    );
    for (const foreground of [
      "color",
      "color-muted",
      "color-faint",
      "primary",
    ]) {
      for (const background of ["bg", "bg-elevated", "sidebar-bg"])
        assert.ok(
          ratio(tokens[foreground], tokens[background]) >= 4.5,
          `${foreground} / ${background}: ${ratio(tokens[foreground], tokens[background]).toFixed(2)}`,
        );
    }
    for (const [foreground, background] of [
      ["primary-color", "primary"],
      ["active-color", "active-bg"],
      ["accent-tag", "bg"],
      ["danger", "danger-bg"],
      ["warning", "warning-bg"],
      ["success", "success-bg"],
    ])
      assert.ok(
        ratio(tokens[foreground], tokens[background]) >= 4.5,
        `${foreground} / ${background}`,
      );
  });
}
