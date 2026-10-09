import { describe, expect, it } from "vitest";
import {
  commandQuery,
  commandWindow,
  matchCommands,
  sessionCommands,
} from "./composer-commands";

describe("slash command suggestions", () => {
  it("only opens on the first line with a collapsed caret at the end of the query", () => {
    expect(commandQuery("/mdl\nKeep this text", 4, 4)).toBe("/mdl");
    expect(commandQuery(" /mdl", 5, 5)).toBeUndefined();
    expect(commandQuery("Text\n/mdl", 9, 9)).toBeUndefined();
    expect(commandQuery("/model", 2, 2)).toBeUndefined();
    expect(commandQuery("/model", 1, 6)).toBeUndefined();
  });
  it("matches subsequences, prefers exact/prefix results and rejects unrelated commands", () => {
    const commands = sessionCommands("claude");
    expect(
      matchCommands(commands, "/mdl").map((match) => match.command.name),
    ).toEqual(["/model"]);
    expect(matchCommands(commands, "/MODEL")[0].command.name).toBe("/model");
    expect(matchCommands(commands, "/mdl")[0].positions).toEqual([0, 1, 3, 5]);
    expect(matchCommands(commands, "/zzz")).toEqual([]);
    expect(
      matchCommands(commands, "/co").every((match) =>
        match.command.name.startsWith("/co"),
      ),
    ).toBe(true);
  });
  it("keeps the selected command at the center and wraps neighbors without duplicating short lists", () => {
    const matches = matchCommands(sessionCommands("claude"), "/");
    const rows = commandWindow(matches, 0);
    expect(rows[3]?.command.name).toBe("/model");
    expect(rows.filter(Boolean)).toHaveLength(matches.length);
    expect(
      new Set(rows.filter(Boolean).map((match) => match!.command.name)).size,
    ).toBe(matches.length);
    expect(commandWindow(matches, matches.length - 1)[3]).toBe(matches.at(-1));
    expect(commandWindow([], 0)).toEqual([]);
  });
  it("offers only reported model/effort values and agent-supported context commands", () => {
    const commands = sessionCommands("codex", {
      model: "test-model",
      effective_model: "test-model",
      models: [
        {
          id: "test-model",
          name: "Test model",
          default: true,
          efforts: ["low", "high"],
          default_effort: "low",
        },
      ],
    });
    const names = commands.map((command) => command.name);
    expect(names).toContain("/model test-model");
    expect(names).toContain("/model default");
    expect(names).toContain("/effort high");
    expect(names).toContain("/effort default");
    expect(names).not.toContain("/effort medium");
    expect(names).not.toContain("/context");
  });
});
