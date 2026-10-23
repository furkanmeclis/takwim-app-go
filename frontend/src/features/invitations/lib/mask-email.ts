/**
 * Same masking as the API preview (`ayse.kaya@example.com` →
 * `a********@e******.com`), so the invite page can tell early that the
 * signed-in account is not the invited one. The API still decides on accept.
 */
export function maskEmail(email: string): string {
  const value = email.trim().toLowerCase();
  const at = value.lastIndexOf("@");
  if (at <= 0) return "***";
  const local = value.slice(0, at);
  const domain = value.slice(at + 1);
  const dot = domain.lastIndexOf(".");
  const host = dot > 0 ? domain.slice(0, dot) : domain;
  const tld = dot > 0 ? domain.slice(dot) : "";
  const mask = (part: string) => {
    const chars = Array.from(part);
    if (chars.length <= 1) return "*";
    return chars[0] + "*".repeat(chars.length - 1);
  };
  return `${mask(local)}@${mask(host)}${tld}`;
}
