const FOLD: Record<string, string> = {
  ç: "c",
  ğ: "g",
  ı: "i",
  ö: "o",
  ş: "s",
  ü: "u",
  â: "a",
  î: "i",
  û: "u",
};

/**
 * Slug preview from an organization name, same rules as the API
 * (`slug.FromName`): Turkish letters folded to ASCII, other characters
 * outside a-z / 0-9 become "-". "Örnek Güzellik Salonu" → "ornek-guzellik-salonu".
 */
export function slugFromName(name: string): string {
  const folded = name
    .trim()
    .replace(/İ/g, "i")
    .replace(/I/g, "i")
    .toLowerCase()
    .replace(/i̇/g, "i")
    .replace(/[çğıöşüâîû]/g, (ch) => FOLD[ch] ?? ch);
  const slug = folded
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64)
    .replace(/-+$/g, "");
  return slug;
}
