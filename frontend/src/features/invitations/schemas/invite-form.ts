import { z } from "zod";

import type { CreateInvitationRequest } from "@/features/invitations/types";

type Translate = (key: string) => string;

export function createInviteSchema(t: Translate) {
  return z
    .object({
      email: z
        .string()
        .trim()
        .min(1, t("invitations.validation.email_required"))
        .email(t("invitations.validation.email_invalid")),
      role_uuids: z
        .array(z.string())
        .min(1, t("invitations.validation.roles_required")),
      branch_scope: z.enum(["all", "selected"]),
      branch_uuids: z.array(z.string()),
    })
    .superRefine((values, ctx) => {
      if (values.branch_scope === "selected" && !values.branch_uuids.length) {
        ctx.addIssue({
          code: "custom",
          path: ["branch_uuids"],
          message: t("invitations.validation.branches_required"),
        });
      }
    });
}

export type InviteFormValues = z.infer<ReturnType<typeof createInviteSchema>>;

export function inviteDefaults(
  branchScope: "all" | "selected" = "all",
): InviteFormValues {
  return {
    email: "",
    role_uuids: [],
    branch_scope: branchScope,
    branch_uuids: [],
  };
}

export function inviteRequest(
  values: InviteFormValues,
): CreateInvitationRequest {
  return {
    email: values.email.trim(),
    role_uuids: values.role_uuids,
    branch_scope: values.branch_scope,
    ...(values.branch_scope === "selected"
      ? { branch_uuids: values.branch_uuids }
      : {}),
  };
}
