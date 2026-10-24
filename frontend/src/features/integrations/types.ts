import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type IntegrationAccount = Schemas["IntegrationAccount"];
export type IntegrationAccountPage =
  Schemas["EnvelopeIntegrationAccountPage"]["data"];
export type IntegrationProvider = Schemas["IntegrationProvider"];
export type IntegrationsMeta = Schemas["EnvelopeIntegrationsMeta"]["data"];
export type IntegrationJobPage = Schemas["EnvelopeIntegrationJobPage"]["data"];
export type IntegrationJob = Schemas["IntegrationJob"];
export type IntegrationJobStatus = Schemas["IntegrationJobStatus"];
export type IntegrationJobDetail = Schemas["IntegrationJobDetail"];
export type IntegrationAttempt = Schemas["IntegrationAttempt"];
export type IntegrationJobRetry =
  Schemas["EnvelopeIntegrationJobRetry"]["data"];
export type IntegrationProviderCode = IntegrationAccount["provider"];

export type ConnectIntegrationRequest = {
  kind?: IntegrationAccount["kind"];
  provider: IntegrationProviderCode;
  display_name: string;
  mode: "sandbox" | "live";
  credentials: Record<string, string>;
};

export type UpdateIntegrationRequest = {
  display_name?: string | null;
  mode?: "sandbox" | "live" | null;
  credentials?: Record<string, string> | null;
};

export type ListIntegrationJobsParams = {
  limit: number;
  offset: number;
  sort?: string;
  status?: string;
  kind?: string;
};

export type ListIntegrationsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  /** Integration kinds, comma separated (sms, whatsapp, email, payment, voice_call). */
  kind?: string;
};
