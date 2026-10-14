// Case-insensitive subsequence matching supports both substrings and abbreviations.
export function matchesValueQuery(text: string, query: string): boolean {
  const haystack = text.normalize("NFKC").toLowerCase();
  let from = 0;
  for (const char of query.normalize("NFKC").toLowerCase().replace(/\s/g, "")) {
    const at = haystack.indexOf(char, from);
    if (at < 0) return false;
    from = at + char.length;
  }
  return true;
}
