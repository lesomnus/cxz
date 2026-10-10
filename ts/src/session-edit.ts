import type { Client } from "@connectrpc/connect";
import { AuxKind } from "../gen/cxz/project_svc_pb";
import type { SessionService } from "../gen/cxz/session_svc_pb";
import { ref } from "./connection";
import { t } from "./i18n";

export type SessionField = "name" | "alias";
export function validateSessionField(field: SessionField, value: string) {
  if (field === "name") return value ? undefined : t("Enter a session title.");
  if (
    !/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(value) ||
    value.length < 3 ||
    value.length > 20
  )
    return t(
      "Alias must be 3–20 characters: lowercase letters, digits and single hyphens, beginning with a letter.",
    );
}

export async function editSession(
  client: Pick<Client<typeof SessionService>, "auxRun" | "patch" | "get">,
  id: string,
  field: SessionField,
  value: string,
) {
  const invalid = validateSessionField(field, value);
  if (invalid) throw new Error(invalid);
  const options = { signal: AbortSignal.timeout(20_000) };
  // Name is a runtime-owned title: the manual-title API cancels generation
  // and records manual provenance. Direct name patches are intentionally closed.
  if (field === "name")
    await client.auxRun(
      { ref: ref(id), kinds: [AuxKind.TITLE], text: value },
      options,
    );
  else await client.patch({ ref: ref(id), alias: value }, options);
  return client.get(
    { ref: ref(id), select: { all: true, project: { all: true } } },
    options,
  );
}
