import { describe, expect, it } from "vitest";
import {
  iconDefinitions,
  projectIconDefinitions,
  projectIconNames,
} from "#src/shared/icons/icon.tsx";
import {
  projectIconLabel,
  projectIconName,
  withProjectIcon,
} from "./project-icon";

const project = (labels: Record<string, string>) =>
  ({ labels }) as Parameters<typeof projectIconName>[0];

describe("reading a project's icon", () => {
  it("answers the chosen glyph", () => {
    expect(projectIconName(project({ icon: "leaf" }))).toBe("leaf");
  });

  // Nothing validates a label on the way in: an older build, a hand-edited row
  // or an agent can leave any string here, so an unknown name is no icon rather
  // than a crash or a blank space where a glyph belongs.
  it("treats anything it does not recognize as no icon", () => {
    const unrecognized: Record<string, string>[] = [
      { icon: "dragon" },
      { icon: "" },
      { icon: "LEAF" },
      { icon: "constructor" },
      { icon: "__proto__" },
      { topic: "web" },
      {},
    ];
    for (const labels of unrecognized)
      expect(projectIconName(project(labels))).toBeUndefined();
  });

  it("answers for a project it has not been given", () => {
    expect(projectIconName(undefined)).toBeUndefined();
  });
});

describe("writing a project's icon", () => {
  it("keeps the labels it is not about", () => {
    expect(withProjectIcon({ topic: "web" }, "rocket")).toEqual({
      topic: "web",
      icon: "rocket",
    });
  });

  // A patch replaces the whole map, and a patch carrying an empty map changes
  // nothing -- which is the one thing clearing an icon must not do.
  it("clears with an empty value rather than an empty map", () => {
    const cleared = withProjectIcon({ icon: "leaf", topic: "web" }, undefined);
    expect(cleared).toEqual({ icon: "", topic: "web" });
    expect(Object.keys(cleared).length).toBeGreaterThan(0);
    expect(projectIconName(project(cleared))).toBeUndefined();
  });

  it("writes an icon for a project that had no labels", () => {
    expect(withProjectIcon(undefined, "leaf")).toEqual({ icon: "leaf" });
  });

  it("round trips every name in the palette", () => {
    for (const name of projectIconNames)
      expect(projectIconName(project(withProjectIcon({}, name)))).toBe(name);
  });
});

describe("the palette", () => {
  // Two maps, one renderer. A name in both would resolve to whichever spread
  // last, so a decorative glyph could quietly replace an action's.
  it("shares no name with the semantic icons", () => {
    const shared = projectIconNames.filter((name) => name in iconDefinitions);
    expect(shared).toEqual([]);
  });

  it("is the label key the rest of the app agrees on", () => {
    expect(projectIconLabel).toBe("icon");
  });

  it("draws something for every name", () => {
    for (const name of projectIconNames) {
      const definition = projectIconDefinitions[name];
      expect(definition.viewBox).toBe("0 0 24 24");
      expect(definition.shapes.length).toBeGreaterThan(0);
    }
  });

  // Enough to tell projects apart, few enough to recognize one in a list.
  it("offers a bounded set of choices", () => {
    expect(projectIconNames.length).toBeGreaterThanOrEqual(12);
    expect(projectIconNames.length).toBeLessThanOrEqual(32);
    expect(new Set(projectIconNames).size).toBe(projectIconNames.length);
  });
});
