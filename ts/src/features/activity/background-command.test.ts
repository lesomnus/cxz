import { expect, it } from "vitest";
import { backgroundCommand } from "./background-command";

it("unwraps timeout options and nested shell command operands without evaluating their contents", () => {
  expect(
    backgroundCommand(
      `timeout --kill-after=3s 120s /usr/bin/zsh -lc 'npm run build -- --mode production'`,
    ),
  ).toBe("npm run build -- --mode production");
  expect(
    backgroundCommand(
      `timeout -k 2s --signal TERM --foreground 1.5m bash -o pipefail -c 'printf "%s" "$USER" | tee output'`,
    ),
  ).toBe(`printf "%s" "$USER" | tee output`);
  expect(
    backgroundCommand(
      `gtimeout -k5s -sTERM -- 1h env /bin/sh -c 'timeout 5s go test ./...'`,
    ),
  ).toBe("go test ./...");
  expect(backgroundCommand(`timeout 5 python -c 'print("hello")'`)).toBe(
    `python -c 'print("hello")'`,
  );
});
it("keeps meaningful environment options and extra shell arguments", () => {
  expect(backgroundCommand(`timeout 5s env FOO=bar zsh -c 'echo "$FOO"'`)).toBe(
    `env FOO=bar zsh -c 'echo "$FOO"'`,
  );
  expect(
    backgroundCommand(`timeout 5s bash -c 'printf "%s" "$1"' name value`),
  ).toBe(`bash -c 'printf "%s" "$1"' name value`);
});
it("preserves unknown options, malformed quotes, ordinary commands and empty input", () => {
  for (const command of [
    "timeout --help",
    "timeout --unknown 5s npm test",
    "timeout 5s",
    "timeout someday npm test",
    "timeout 5s bash -c 'unterminated",
    "go test ./...",
    "zsh script.sh",
    "",
  ]) {
    expect(backgroundCommand(command)).toBe(command);
  }
});
