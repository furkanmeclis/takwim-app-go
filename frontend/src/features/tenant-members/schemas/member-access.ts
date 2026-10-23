import { z } from "zod";

import type {
  TenantMember,
  TenantMemberPatchRequest,
} from "@/features/tenant-members/types";

type Translate = (key: string) => string;

export function createMemberAccessSchema(t: Translate) {
  return z
    .object({
      role_uuids: z
        .array(z.string())
        .min(1, t("tenant_members.validation.roles_required")),
      branch_scope: z.enum(["all", "selected"]),
      branch_uuids: z.array(z.string()),
    })
    .superRefine((values, ctx) => {
      if (values.branch_scope === "selected" && !values.branch_uuids.length) {
        ctx.addIssue({
          code: "custom",
          path: ["branch_uuids"],
          message: t("tenant_members.validation.branches_required"),
        });
      }
    });
}

export type MemberAccessValues = z.infer<
  ReturnType<typeof createMemberAccessSchema>
>;

export function memberAccessDefaults(member: TenantMember): MemberAccessValues {
  return {
    role_uuids: member.roles.map((role) => role.uuid),
    branch_scope: member.branch_scope,
    branch_uuids: member.branches.map((branch) => branch.uuid),
  };
}

export function memberAccessRequest(
  values: MemberAccessValues,
): TenantMemberPatchRequest {
  return {
    role_uuids: values.role_uuids,
    branch_scope: values.branch_scope,
    ...(values.branch_scope === "selected"
      ? { branch_uuids: values.branch_uuids }
      : {}),
  };
}
