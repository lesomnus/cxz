import type { Timestamp } from "@bufbuild/protobuf/wkt";

export type DateTimeValue = Timestamp | bigint | number;

export function dateTimeDate(value?: DateTimeValue) {
  if (value === undefined) return undefined;
  const ms =
    typeof value === "object"
      ? Number(value.seconds) * 1000 + value.nanos / 1e6
      : Number(value);
  const date = new Date(ms);
  return Number.isFinite(date.getTime()) ? date : undefined;
}

export {
  formatRelativeDateTime,
  formatLocalDateTime,
  formatOffsetDateTime,
} from "@lesomnus/nox";
