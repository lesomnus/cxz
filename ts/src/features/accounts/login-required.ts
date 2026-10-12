import { Code, ConnectError } from "@connectrpc/connect";
import { BinaryReader, WireType } from "@bufbuild/protobuf/wire";

// Decode the standard google.rpc.ErrorInfo detail, not prose that happens to
// mention login. Only the runtime's explicit missing-profile signal can retry
// a creation. Other authentication failures remain errors.
export function requiredSessionLogin(
  error: unknown,
  account: string,
  creationKey: string,
) {
  if (
    !(error instanceof ConnectError) ||
    error.code !== Code.FailedPrecondition
  )
    return;
  for (const detail of error.details) {
    if (!("type" in detail) || detail.type !== "google.rpc.ErrorInfo") continue;
    try {
      const reader = new BinaryReader(detail.value);
      let reason = "",
        domain = "";
      const metadata: Record<string, string> = {};
      while (reader.pos < reader.len) {
        const [field, wire] = reader.tag();
        if (wire !== WireType.LengthDelimited) {
          reader.skip(wire, field);
          continue;
        }
        if (field === 1) reason = reader.string();
        else if (field === 2) domain = reader.string();
        else if (field === 3) {
          const entry = new BinaryReader(reader.bytes());
          let key = "",
            value = "";
          while (entry.pos < entry.len) {
            const [field, wire] = entry.tag();
            if (field === 1 && wire === WireType.LengthDelimited)
              key = entry.string();
            else if (field === 2 && wire === WireType.LengthDelimited)
              value = entry.string();
            else entry.skip(wire, field);
          }
          metadata[key] = value;
        } else reader.skip(wire, field);
      }
      const key = metadata.session_key || creationKey;
      if (
        reason === "PROJECT_LOGIN_REQUIRED" &&
        domain === "cxz.auth" &&
        metadata.account === account &&
        key === creationKey
      )
        return { account, key };
    } catch {
      /* Malformed details cannot authorize a retry. */
    }
  }
}
