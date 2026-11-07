import {
  DateTime as NoxDateTime,
  RelativeTime as NoxRelativeTime,
} from "@lesomnus/nox";
import { dateTimeDate, type DateTimeValue } from "./date-time-format";
export function DateTime({
  value,
  ...props
}: {
  value?: DateTimeValue;
  title?: string;
  showOffset?: boolean;
}) {
  return <NoxDateTime {...props} value={dateTimeDate(value)} empty="—" />;
}
export function RelativeTime({ value }: { value?: DateTimeValue }) {
  return <NoxRelativeTime value={dateTimeDate(value)} />;
}
