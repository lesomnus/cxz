import { Code, ConnectError } from "@connectrpc/connect";
import { BinaryReader, WireType } from "@bufbuild/protobuf/wire";

export type ErrorInfo = {
  reason: string;
  domain: string;
  metadata: Record<string, string>;
};

// The standard google.rpc.ErrorInfo details of a FailedPrecondition, decoded
// by hand: the schema is three fields and not worth generating. A detail that
// does not decode is left out -- it cannot be the signal anything waits for.
export function errorInfos(error: unknown): ErrorInfo[] {
  if (
    !(error instanceof ConnectError) ||
    error.code !== Code.FailedPrecondition
  )
    return [];
  const infos: ErrorInfo[] = [];
  for (const detail of error.details) {
    if (!("type" in detail) || detail.type !== "google.rpc.ErrorInfo") continue;
    try {
      const reader = new BinaryReader(detail.value);
      const info: ErrorInfo = { reason: "", domain: "", metadata: {} };
      while (reader.pos < reader.len) {
        const [field, wire] = reader.tag();
        if (wire !== WireType.LengthDelimited) {
          reader.skip(wire, field);
          continue;
        }
        if (field === 1) info.reason = reader.string();
        else if (field === 2) info.domain = reader.string();
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
          info.metadata[key] = value;
        } else reader.skip(wire, field);
      }
      infos.push(info);
    } catch {
      /* Malformed details cannot authorize a retry. */
    }
  }
  return infos;
}
