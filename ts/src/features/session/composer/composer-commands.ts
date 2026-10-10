import { t } from "#src/shared/i18n/i18n.ts";
import { selectedModel, type ModelCatalog } from "./model-settings";

export type ComposerCommand = { name: string; description: string };
export type CommandMatch = {
  command: ComposerCommand;
  score: number;
  positions: number[];
};
export const COMMAND_NEIGHBORS = 3;

export function sessionCommands(
  agent: string,
  catalog?: ModelCatalog,
): ComposerCommand[] {
  const commands: ComposerCommand[] = [
    { name: "/model", description: t("Choose the provider model") },
    { name: "/effort", description: t("Choose the reasoning effort") },
    { name: "/compact", description: t("Compact the current context") },
  ];
  if (agent === "claude")
    commands.push({
      name: "/context",
      description: t("Inspect the current context"),
    });
  if (!catalog) return commands;
  const models = [...catalog.models];
  if (
    models.some((model) => model.default) &&
    !models.some((model) => model.id === "default")
  )
    commands.push({
      name: "/model default",
      description: t("Use the default model"),
    });
  for (const model of models)
    commands.push({
      name: `/model ${model.id}`,
      description:
        typeof model.name === "string" && model.name
          ? model.name
          : t("Choose the provider model"),
    });
  const selected = selectedModel(catalog);
  const efforts = (selected?.efforts ?? []).filter(
    (value) => typeof value === "string" && /^\S+$/.test(value),
  );
  if (efforts.length && (agent === "claude" || selected?.default_effort))
    efforts.push("default");
  for (const effort of new Set(efforts))
    commands.push({
      name: `/effort ${effort}`,
      description: t("Set reasoning effort to {effort}", { effort }),
    });
  return commands;
}

export function commandQuery(value: string, start: number, end: number) {
  const newline = value.indexOf("\n");
  const first = newline < 0 ? value : value.slice(0, newline);
  if (
    !first.startsWith("/") ||
    start !== end ||
    start !== first.length ||
    first.includes("\t")
  )
    return;
  return first;
}

// Subsequence matching keeps tight matches ahead of scattered ones, while exact
// and prefix matches win. Descriptions explain commands, not hidden aliases.
export function matchCommands(
  commands: readonly ComposerCommand[],
  query: string,
): CommandMatch[] {
  const needle = query.toLowerCase();
  return commands
    .flatMap((command) => {
      const name = command.name.toLowerCase();
      const positions: number[] = [];
      let offset = 0,
        gaps = 0;
      for (const char of needle) {
        const index = name.indexOf(char, offset);
        if (index < 0) return [];
        positions.push(index);
        gaps += index - offset;
        offset = index + 1;
      }
      const score =
        name === needle ? 0 : name.startsWith(needle) ? 1 : 2 + gaps;
      return [{ command, positions, score }];
    })
    .sort((a, b) => a.score - b.score);
}

export function commandWindow(matches: CommandMatch[], selected: number) {
  if (!matches.length) return [];
  const before = Math.min(
    COMMAND_NEIGHBORS,
    Math.floor((matches.length - 1) / 2),
  );
  const after = Math.min(COMMAND_NEIGHBORS, matches.length - before - 1);
  return Array.from({ length: 2 * COMMAND_NEIGHBORS + 1 }, (_, row) => {
    const distance = row - COMMAND_NEIGHBORS;
    if (distance < -before || distance > after) return undefined;
    return matches[(selected + distance + matches.length) % matches.length];
  });
}
