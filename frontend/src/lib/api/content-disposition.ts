/** Filename from a Content-Disposition header (RFC 5987 `filename*` first). */
export function parseContentDispositionFilename(
  header: string | null,
): string | null {
  if (!header) return null;
  const star = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (star?.[1]) {
    try {
      return decodeURIComponent(star[1].trim());
    } catch {
      return star[1].trim();
    }
  }
  const plain = /filename="([^"]+)"/i.exec(header);
  if (plain?.[1]) return plain[1];
  const unquoted = /filename=([^;]+)/i.exec(header);
  return unquoted?.[1]?.trim() ?? null;
}
