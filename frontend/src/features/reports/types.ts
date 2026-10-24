import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Report = Schemas["Report"];
export type ReportDay = Schemas["ReportDay"];
export type ReportBranch = Schemas["ReportBranch"];
export type ReportMoney = Schemas["ReportMoney"];
export type ReportStock = Schemas["ReportStock"];
export type ReportReservations = Schemas["ReportReservations"];
export type ReportConfirmations = Schemas["ReportConfirmations"];
export type ReportFigures = Schemas["ReportFigures"];
export type ReportPackages = Schemas["ReportPackages"];
export type ReportPackageBalance = Schemas["ReportPackageBalance"];

/** CSV sections of `GET /v1/tenant/reports/export`. */
export type ReportSection = "days" | "branches";

/** Query of the report endpoints (dates as "2026-06-01"). */
export type ReportFilter = {
  date_from: string;
  date_to: string;
  /** A branch uuid, `head_office`, or empty for every branch. */
  branch?: string;
  /** A staff uuid, or empty for everyone. */
  staff?: string;
};
