/** Translation keys of the seeded tenant template roles. */
const TEMPLATE_ROLE_KEYS: Record<string, string> = {
  owner: "tenant.roles.owner",
  admin: "tenant.roles.admin",
  member: "tenant.roles.member",
};

/** Display name of a tenant role: translated for template roles. */
export function tenantRoleLabel(
  t: (key: string) => string,
  role: { key: string; name?: string } | null | undefined,
): string {
  if (!role) return "";
  const key = TEMPLATE_ROLE_KEYS[role.key];
  if (key) return t(key);
  return role.name || role.key;
}
