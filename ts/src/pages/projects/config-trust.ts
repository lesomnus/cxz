import { ConnectError } from "@connectrpc/connect";
import { errorInfos } from "#src/shared/api/error-info.ts";

// Mirrors internal/configtrust: the manager refuses a configuration that asks
// for something dangerous until a person has said yes to it, and says so with
// this detail. The message lists where each finding is and what it asks for --
// never the values -- between a fixed first and last line.
const prefix = "devcontainer configuration requires explicit trust:";
const suffix = "inspect these settings and explicitly pass --trust-config";

// The findings a person is being asked to accept, or undefined when the error
// is anything else. Only the structured detail asks; prose that happens to
// mention trust is just an error.
export function trustRequired(error: unknown): string[] | undefined {
  if (
    !errorInfos(error).some(
      (v) => v.domain === "cxz.config" && v.reason === "TRUST_REQUIRED",
    )
  )
    return;
  const lines = (error as ConnectError).rawMessage.split("\n");
  const start = lines.findIndex((line) => line.endsWith(prefix));
  const end = lines.findIndex((line) => line.startsWith(suffix));
  return lines
    .slice(start + 1, end < 0 ? undefined : end)
    .map((line) => line.trim())
    .filter(Boolean);
}
