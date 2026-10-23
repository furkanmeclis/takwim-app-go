import { z } from "zod";

type Translate = (key: string) => string;

const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function createOrganizationSchema(t: Translate) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, t("organizations.validation.name_required"))
        .max(255, t("organizations.validation.name_too_long")),
      slug: z
        .string()
        .trim()
        .max(64, t("organizations.validation.slug_invalid"))
        .refine(
          (value) => value === "" || SLUG_RE.test(value),
          t("organizations.validation.slug_invalid"),
        ),
      owner_mode: z.enum(["existing", "invite"]),
      owner_user_uuid: z.string(),
      owner_email: z.string().trim(),
    })
    .superRefine((values, ctx) => {
      if (values.owner_mode === "existing" && !values.owner_user_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["owner_user_uuid"],
          message: t("organizations.validation.owner_required"),
        });
      }
      if (values.owner_mode === "invite") {
        const email = z.string().email().safeParse(values.owner_email);
        if (!email.success) {
          ctx.addIssue({
            code: "custom",
            path: ["owner_email"],
            message: t("organizations.validation.owner_email_invalid"),
          });
        }
      }
    });
}

export type OrganizationFormValues = z.infer<
  ReturnType<typeof createOrganizationSchema>
>;

export function createOrganizationEditSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("organizations.validation.name_required"))
      .max(255, t("organizations.validation.name_too_long")),
    slug: z
      .string()
      .trim()
      .min(1, t("organizations.validation.slug_invalid"))
      .max(64, t("organizations.validation.slug_invalid"))
      .regex(SLUG_RE, t("organizations.validation.slug_invalid")),
  });
}

export type OrganizationEditValues = z.infer<
  ReturnType<typeof createOrganizationEditSchema>
>;
