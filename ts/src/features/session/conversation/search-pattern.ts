export type SearchOptions = {
  matchCase: boolean;
  wholeWord: boolean;
  useRegex: boolean;
};

export const defaultSearchOptions: SearchOptions = {
  matchCase: false,
  wholeWord: false,
  useRegex: false,
};

export function compileSearchPattern(
  query: string,
  options = defaultSearchOptions,
): { pattern?: RegExp; error?: string } {
  if (!query) return {};
  const expression = options.useRegex
    ? query
    : query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  try {
    const flags = options.matchCase ? "gmu" : "gimu";
    const pattern = new RegExp(expression, flags);
    if (!options.wholeWord) return { pattern };
    const word = String.raw`[\p{L}\p{N}\p{M}_]`;
    return {
      pattern: new RegExp(
        `(?<!${word})(?:${pattern.source})(?!${word})`,
        flags,
      ),
    };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}

export function* searchMatches(text: string, pattern?: RegExp) {
  if (!pattern) return;
  for (const match of text.matchAll(pattern)) {
    // Empty matches have no visible text to highlight or navigate to.
    if (match[0].length) yield match;
  }
}
