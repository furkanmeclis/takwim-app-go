import { z } from "zod";

import type {
  TenantRole,
  TenantRoleRequest,
} from "@/features/tenant-roles/types";

type Translate = (key: string) => string;

export const ROLE_KEY_PATTERN = /^[a-z][a-z0-9_]{1,63}$/;

export function createTenantRoleSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("tenant_roles.validation.name_required"))
      .max(100, t("tenant_roles.validation.name_too_long")),
    key: z
      .string()
      .trim()
      .refine(
        (value) => value === "" || ROLE_KEY_PATTERN.test(value),
        t("tenant_roles.validation.key_invalid"),
      ),
    description: z
      .string()
      .max(500, t("tenant_roles.validation.description_too_long")),
    permissions: z.array(z.string()),
  });
}

export type TenantRoleFormValues = z.infer<
  ReturnType<typeof createTenantRoleSchema>
>;

/** Lowercase key from a (Turkish) display name: "Gece Vardiyası" → "gece_vardiyasi". */
export function roleKeyFromName(name: string): string {
  const map: Record<string, string> = {
    ç: "c",
    ğ: "g",
    ı: "i",
    i̇: "i",
    ö: "o",
    ş: "s",
    ü: "u",
  };
  return name
    .trim()
    .toLocaleLowerCase("tr")
    .replace(/[çğıöşü]/g, (ch) => map[ch] ?? ch)
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 64);
}

export function tenantRoleDefaults(role?: TenantRole): TenantRoleFormValues {
  return {
    name: role?.name ?? "",
    key: role?.key ?? "",
    description: role?.description ?? "",
    permissions: role?.permissions ?? [],
  };
}

export function tenantRoleRequest(
  values: TenantRoleFormValues,
  mode: "create" | "edit",
): TenantRoleRequest {
  return {
    name: values.name.trim(),
    description: values.description.trim(),
    permissions: values.permissions,
    ...(mode === "create" && values.key ? { key: values.key } : {}),
  };
}
