/** True when pathname is the base path or lies under it; the root path matches only itself. */
export function pathMatches(pathname: string, base: string): boolean {
  if (base === '/') return pathname === '/';
  return pathname === base || pathname.startsWith(`${base}/`);
}
