import type { Client } from "@connectrpc/connect";
import type { SessionService } from "../../../../gen/cxz/session_svc_pb";
import { ref } from "../../../shared/api/connection";
import { t } from "../../../shared/i18n/i18n";

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
  client: Pick<Client<typeof SessionService>, "patch" | "get">,
  id: string,
  field: SessionField,
  value: string,
) {
  const invalid = validateSessionField(field, value);
  if (invalid) throw new Error(invalid);
  const options = { signal: AbortSignal.timeout(20_000) };
  // Patch records manual-title provenance as well as resource metadata.
  await client.patch({ ref: ref(id), [field]: value }, options);
  return client.get(
    { ref: ref(id), select: { all: true, project: { all: true } } },
    options,
  );
}
