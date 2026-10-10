import { useLocale } from "#src/shared/i18n/i18n-react.tsx";

export function ResourceIcon({
  kind,
}: {
  kind: "sessions" | "projects" | "settings";
}) {
  useLocale();
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {kind === "sessions" ? (
        <path d="M5 4h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-6 3V6a2 2 0 0 1 2-2Z" />
      ) : kind === "projects" ? (
        <path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" />
      ) : (
        <>
          <circle cx="12" cy="12" r="3" />
          <path d="M9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1Z" />
        </>
      )}
    </svg>
  );
}
