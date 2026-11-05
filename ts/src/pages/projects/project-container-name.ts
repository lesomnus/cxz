export function containerNamePrefixLength(name?: string) {
  const installationPrefix = /^cxz-[0-9a-f]{12}-/i.exec(name ?? "")?.[0];
  return (installationPrefix?.length ?? 0) + 7;
}
