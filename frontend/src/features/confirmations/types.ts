import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ConfirmationRule = Schemas["ConfirmationRule"];
export type ConfirmationSaveInput = Schemas["ConfirmationSaveInput"];
export type ConfirmationBranchRule = Schemas["ConfirmationBranchRule"];
export type ConfirmationSettings = Schemas["ConfirmationSettings"];
export type ConfirmationLimits = ConfirmationSettings["limits"];
export type OnNoAnswer = ConfirmationRule["on_no_answer"];

/** The rule being edited: the business-wide one (branch null) or a branch's. */
export type ConfirmationRuleTarget = {
  branch: ConfirmationBranchRule["branch"] | null;
  rule: ConfirmationRule;
  custom: boolean;
};
