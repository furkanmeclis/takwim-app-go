import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type SectorCode = Schemas["SectorCode"];
export type SectorModule = Schemas["SectorModule"];
export type Sector = Schemas["Sector"];
export type SectorStep = Sector["flow"][number];
export type SectorResourceKind = Sector["capacity"]["resources"][number];
