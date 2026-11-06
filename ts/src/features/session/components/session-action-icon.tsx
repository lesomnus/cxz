import { Icon } from "#src/shared/icons/icon.tsx";
export function SessionActionIcon({
  action,
}: {
  action:
    | "stop"
    | "resume"
    | "remove"
    | "restart"
    | "fork"
    | "erase"
    | "details"
    | "terminal"
    | "upload"
    | "folder";
}) {
  return <Icon name={action} />;
}
