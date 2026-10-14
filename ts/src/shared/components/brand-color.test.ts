import { expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { BRAND_COLOR } from "../../../packages/nox/src/theme/colors";

it("keeps CSS and generated-image brand colors identical", () => {
  const tokens = readFileSync(
    new URL("../../../packages/nox/src/theme/tokens.css", import.meta.url),
    "utf8",
  );
  expect(tokens.match(/--ui-brand-color:\s*([^;]+);/)?.[1]).toBe(BRAND_COLOR);
});
