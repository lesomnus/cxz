import { useEffect, useRef, useState } from "react";
import type { Connection } from "#src/shared/api/connection.ts";
import { useProjectConnection } from "#src/shared/api/project-connection.ts";
import { Icon, type IconProps } from "#src/shared/icons/icon.tsx";

const backends = new Map<string, IconProps["name"]>([
  ["docker", "connection-docker"],
  ["kubernetes", "connection-kubernetes"],
]);

const transports = new Map<string, IconProps["name"]>([
  ["local", "connection-local"],
  ["unix", "connection-local"],
  ["ssh", "terminal"],
  ["origin", "globe"],
  ["connect", "globe"],
]);

export function ConnectionIcon({
  c,
  baseUrl,
  backend,
  transport = "origin",
}: {
  c: Connection;
  baseUrl?: string;
  backend?: string;
  transport?: string;
}) {
  const icon = useRef<SVGSVGElement>(null);
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    if (!baseUrl || visible || !icon.current) return;
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) setVisible(true);
    });
    observer.observe(icon.current);
    return () => observer.disconnect();
  }, [baseUrl, visible]);
  const { data } = useProjectConnection(c, visible ? baseUrl : undefined, {
    cached: true,
  });
  const runtime = data && data.baseUrl === baseUrl ? data.backend : undefined;
  const name =
    backends.get(backend ?? runtime ?? "") ??
    transports.get(transport) ??
    "connection-gateway";
  return <Icon ref={icon} name={name} size="var(--ui-icon-size-small)" />;
}
