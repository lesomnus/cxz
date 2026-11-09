export type ShellWord = { value: string; start: number; end: number };

// Parse shell quoting for display only. Do not evaluate variables, substitutions
// or scripts. Unrecognized/ambiguous wrappers retain the full native command.
export function shellWords(text: string): ShellWord[] | undefined {
  const words: ShellWord[] = [];
  let word = "",
    quote = "",
    started = false,
    start = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (!started && !/\s/.test(c)) start = i;
    if (c === "\\" && quote !== "'") {
      const next = text[++i];
      if (next === undefined) return;
      if (next !== "\n")
        word += quote === '"' && !'$`"\\'.includes(next) ? "\\" + next : next;
      started = true;
    } else if (quote) {
      if (c === quote) quote = "";
      else word += c;
    } else if (c === "'" || c === '"') {
      quote = c;
      started = true;
    } else if (/\s/.test(c)) {
      if (started) words.push({ value: word, start, end: i });
      word = "";
      started = false;
    } else {
      word += c;
      started = true;
    }
  }
  if (quote) return;
  if (started) words.push({ value: word, start, end: text.length });
  return words;
}

export function shellCommand(command: string) {
  const original = { shell: "", command };
  const words = shellWords(command)?.map((word) => word.value);
  if (!words || words.length < 3) return original;
  const shell = words[0].split("/").at(-1)!;
  if (!["sh", "bash", "zsh", "dash", "ksh", "ash"].includes(shell))
    return original;
  let carries = false;
  for (let i = 1; i < words.length - 1; i++) {
    const option = words[i];
    if (option === "-o" || option === "+o") i++;
    else if (option.startsWith("--")) continue;
    else if (option.startsWith("-")) carries ||= option.slice(1).includes("c");
    else return original;
  }
  const script = words.at(-1)!;
  return carries && script.trim() ? { shell, command: script } : original;
}
