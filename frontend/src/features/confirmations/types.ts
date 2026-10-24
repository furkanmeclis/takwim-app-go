import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ConfirmationRule = Schemas["ConfirmationRule"];
export type ConfirmationSaveInput = Schemas["ConfirmationSaveInput"];
export type ConfirmationBranchRule = Schemas["ConfirmationBranchRule"];
export type ConfirmationSettings = Schemas["ConfirmationSettings"];
export type ConfirmationLimits = ConfirmationSettings["limits"];
export type OnNoAnswer = ConfirmationRule["on_no_answer"];
/** The business's SMS and e-mail texts of the request (TEC-979). */
export type ConfirmationTexts = Schemas["ConfirmationTexts"];
export type ConfirmationTextsInput = Schemas["ConfirmationTextsInput"];

/** The rule being edited: the business-wide one (branch null) or a branch's. */
export type ConfirmationRuleTarget = {
  branch: ConfirmationBranchRule["branch"] | null;
  rule: ConfirmationRule;
  custom: boolean;
};

/** One booking of the confirmation list (TEC-924). */
export type ConfirmationTask = Schemas["ConfirmationTask"];
export type ConfirmationTaskPage = Schemas["ConfirmationTaskPage"];
export type ConfirmationState = Schemas["ConfirmationState"];
export type ConfirmationFacets = Schemas["ConfirmationFacets"];
export type ConfirmationSummary = Schemas["ConfirmationSummary"];
export type ConfirmationAttempt = Schemas["ConfirmationAttempt"];
export type ConfirmationConfirmInput = Schemas["ConfirmationConfirmInput"];

export type ListConfirmationsParams = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
  branch?: string;
  state?: string;
  attention?: "true" | "false";
  starts_from?: string;
  starts_to?: string;
};

export type ConfirmationFacetParams = Pick<
  ListConfirmationsParams,
  "q" | "branch" | "starts_from" | "starts_to"
>;
