/**
 * Turkish tax identity checks, mirrored from the API
 * (`orgsettings/usecase/validate.go`) so the form flags typos before saving.
 */

/** Vergi Kimlik Numarası: 10 digits, GİB checksum. */
export function isValidVKN(value: string): boolean {
  if (!/^\d{10}$/.test(value)) return false;
  let sum = 0;
  for (let i = 0; i < 9; i++) {
    const tmp = (Number(value[i]) + 9 - i) % 10;
    if (tmp === 0) continue;
    let p = (tmp * 2 ** (9 - i)) % 9;
    if (p === 0) p = 9;
    sum += p;
  }
  return (10 - (sum % 10)) % 10 === Number(value[9]);
}

/** T.C. Kimlik No: 11 digits, first not zero, two check digits. */
export function isValidTCKN(value: string): boolean {
  if (!/^[1-9]\d{10}$/.test(value)) return false;
  const d = [...value].map(Number);
  const odd = d[0] + d[2] + d[4] + d[6] + d[8];
  const even = d[1] + d[3] + d[5] + d[7];
  if ((((odd * 7 - even) % 10) + 10) % 10 !== d[9]) return false;
  const total = d.slice(0, 10).reduce((acc, n) => acc + n, 0);
  return total % 10 === d[10];
}
