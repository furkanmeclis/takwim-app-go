export function isNavHrefActive(
  pathname: string,
  href: string,
  homeHref: string,
): boolean {
  return (
    pathname === href || (href !== homeHref && pathname.startsWith(`${href}/`))
  );
}

export function formatNavCount(value: number, max = 99): string {
  if (value > max) return `${max}+`;
  return String(value);
}
