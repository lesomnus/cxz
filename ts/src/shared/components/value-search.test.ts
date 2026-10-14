import { describe, expect, it } from "vitest";
import { matchesValueQuery } from "../../../packages/ui/src/components/value-menu/value-search";
describe("project query matching", () => {
  it("matches substrings, ordered abbreviations, unicode and case-insensitive queries", () => {
    for (const query of ["", "Demo", "PROJECT", "dmprj", "d p j", "ＤＥＭＯ"])
      expect(matchesValueQuery("Demo project", query)).toBe(true);
    expect(matchesValueQuery("프로젝트", "프젝")).toBe(true);
    expect(matchesValueQuery("Demo project", "jpd")).toBe(false);
    expect(matchesValueQuery("Demo project", "missing")).toBe(false);
  });
});
