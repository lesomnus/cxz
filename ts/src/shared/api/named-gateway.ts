import { t } from "#src/shared/i18n/i18n.ts";

export function namedGatewayUrl(name: string) {
  return /^[a-z0-9][a-z0-9-]{0,39}$/.test(name)
    ? `${location.origin}/c/${name}`
    : undefined;
}

export async function namedGatewayStatus(baseUrl: string, signal: AbortSignal) {
  const response = await fetch(`${baseUrl}/auth/status`, { signal });
  if (
    !response.ok ||
    response.headers.get("content-type")?.includes("text/html")
  )
    throw new Error(t("Cannot reach cxz"));
  return response.headers.get("payday-identity") ?? undefined;
}
