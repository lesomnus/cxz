import { shellCommand, shellWords } from "#src/shared/editor/shell-command.ts";

const duration = /^(?:\d+(?:\.\d*)?|\.\d+)(?:[smhd])?$/;
const executable = (word: string) => word.split("/").at(-1);

/** Display-only heuristic; original commands remain stored and available on hover. */
export function backgroundCommand(original: string) {
  let command = original.trim();
  for (let depth = 0; depth < 8; depth++) {
    const words = shellWords(command);
    if (!words?.length) break;
    const name = executable(words[0].value);
    if (name === "timeout" || name === "gtimeout") {
      let index = 1;
      for (; index < words.length; index++) {
        const option = words[index].value;
        if (option === "--") {
          index++;
          break;
        }
        if (
          ["--foreground", "--preserve-status", "--verbose", "-v"].includes(
            option,
          )
        )
          continue;
        if (["-k", "--kill-after", "-s", "--signal"].includes(option)) {
          if (!words[++index]) return command;
          continue;
        }
        if (/^(?:--(?:kill-after|signal)=.+|-[ks].+)$/.test(option)) continue;
        break;
      }
      if (
        !words[index] ||
        !duration.test(words[index].value) ||
        !words[index + 1]
      )
        break;
      command = command.slice(words[index + 1].start);
      continue;
    }
    // Keep environment assignments/options; they can materially change the job.
    if (
      name === "env" &&
      words[1] &&
      !words[1].value.startsWith("-") &&
      !words[1].value.includes("=")
    ) {
      command = command.slice(words[1].start);
      continue;
    }
    const shell = shellCommand(command);
    if (!shell.shell || shell.command === command) break;
    command = shell.command.trim();
  }
  return command || original;
}
