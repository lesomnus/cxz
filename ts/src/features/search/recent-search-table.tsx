import {
  Button,
  RelativeTime,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@lesomnus/nox";
import { formatOffsetDateTime } from "#src/shared/components/date-time-format.ts";
import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import resourceTableStyles from "#src/shared/components/resource-table.module.css";
import type { RecentSearch } from "./recent-searches";
import styles from "./recent-search-table.module.css";

export function RecentSearchTable({
  recent,
  repeat,
  clear,
}: {
  recent: RecentSearch[];
  repeat: (search: RecentSearch) => void;
  clear: () => void;
}) {
  return (
    <section>
      <header className={styles.header}>
        <h2>{t("Recent searches")}</h2>
        <Button type="button" variant="toolbar" onClick={clear}>
          {t("Clear history")}
        </Button>
      </header>
      <Table
        aria-label={t("Recent searches")}
        stickyHeader={false}
        className={`${resourceTableStyles.layout} ${resourceTableStyles.table} ${styles.table}`}
      >
        <colgroup>
          <col className={styles.queryColumn} />
          <col />
          <col className={styles.resultsColumn} />
          <col className={styles.timeColumn} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead>{t("Search")}</TableHead>
            <TableHead>{t("Filters")}</TableHead>
            <TableHead align="right">{t("Results")}</TableHead>
            <TableHead>{t("Time")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {recent.map((entry) => (
            <TableRow key={entry.id}>
              <TableCell className={styles.queryCell}>
                <Button
                  type="button"
                  rowAware
                  className={styles.query}
                  title={t("Repeat this search")}
                  onClick={() => repeat(entry)}
                >
                  {entry.query}
                </Button>
              </TableCell>
              <TableCell className={styles.filters}>
                {entry.projectName || entry.project || t("All projects")}
                <small>
                  {entry.match === "regex"
                    ? t("Regular expression")
                    : entry.match === "fuzzy"
                      ? t("Fuzzy search")
                      : t("Substring")}
                  {" · "}
                  {entry.ignoreCase ? t("Case insensitive") : t("Match case")}
                </small>
                <small>
                  {entry.since && entry.until
                    ? `${entry.since} – ${entry.until} UTC`
                    : entry.since
                      ? t("From {date} (UTC)", { date: entry.since })
                      : entry.until
                        ? t("Before {date} (UTC)", { date: entry.until })
                        : t("No date filters")}
                </small>
              </TableCell>
              <TableCell align="right" className={styles.results}>
                {entry.hits.toLocaleString(currentLocale())}
                {entry.hasMore && "+"}
                {entry.partial && <small>{t("Partial results")}</small>}
              </TableCell>
              <TableCell className={styles.time}>
                <RelativeTime
                  compact
                  value={entry.time}
                  title={formatOffsetDateTime(new Date(entry.time))}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </section>
  );
}
