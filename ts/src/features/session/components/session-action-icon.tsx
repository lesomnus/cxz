export function SessionActionIcon({
  action,
}: {
  action:
    | "stop"
    | "resume"
    | "restart"
    | "fork"
    | "purge"
    | "details"
    | "terminal"
    | "upload"
    | "folder";
}) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {action === "stop" && <rect x="4" y="4" width="8" height="8" rx="1" />}
      {action === "resume" && <path d="m5 3 7 5-7 5Z" />}
      {action === "restart" && (
        <>
          <path d="M13 7a5 5 0 1 0-.8 3.7M13 3v4H9" />
        </>
      )}
      {action === "fork" && (
        <>
          <path d="M5 11V5m0 4h3a3 3 0 0 0 3-3V5" />
          <circle cx="5" cy="3" r="1.5" />
          <circle cx="11" cy="3" r="1.5" />
          <circle cx="5" cy="13" r="1.5" />
        </>
      )}
      {action === "purge" && (
        <>
          <path d="M3 4h10M6 4V2h4v2M4 4l.7 9h6.6l.7-9M6.5 6.5v4M9.5 6.5v4" />
        </>
      )}
      {action === "details" && (
        <>
          <circle cx="8" cy="8" r="5.5" />
          <path d="M8 7.5v3M8 5.2v.1" />
        </>
      )}
      {action === "upload" && <path d="M8 11V2m-3 3 3-3 3 3M3 10v4h10v-4" />}
      {action === "terminal" && <path d="m3 4 4 4-4 4m6 0h4" />}
      {action === "folder" && (
        <path d="M2 4h5l1.5 2H14v7H2ZM8 11V7m-2 2 2-2 2 2" />
      )}
    </svg>
  );
}
