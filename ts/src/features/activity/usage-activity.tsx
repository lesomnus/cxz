import { ActivityHeatmap } from "@lesomnus/nox";
import type { ConversationStatsReply } from "#gen/cxz/session_svc_pb";
import {
  usageActivityDays,
  usageActivityRange,
  type ActivityDay,
} from "./usage-activity-data";
export function UsageActivity({
  stats,
  loading,
  weeks,
  toMs,
  selectedDate,
  onSelect,
  onWeeksChange,
}: {
  stats?: ConversationStatsReply;
  loading: boolean;
  weeks: number;
  toMs: bigint;
  selectedDate: string;
  onSelect: (day: ActivityDay) => void;
  onWeeksChange: (weeks: number) => void;
}) {
  const days = usageActivityDays(
    stats ?? { ...usageActivityRange(weeks, toMs), days: [] },
  ).map((day) => ({
    ...day,
    label: `${day.date} (UTC): ${day.values?.finishedTurns ?? 0n} finished turns`,
  }));
  return (
    <ActivityHeatmap
      days={days}
      loading={loading}
      placeholder={!stats}
      selectedDate={selectedDate}
      onSelect={onSelect}
      onWeeksChange={onWeeksChange}
      moreLabel="More turns"
      legendLabel="Color intensity: finished turns per day"
    />
  );
}
