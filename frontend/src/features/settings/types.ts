import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type SystemInfo = Schemas["SystemInfo"];
export type RuntimeInfo = Schemas["SystemRuntimeInfo"];
export type MemoryStats = Schemas["SystemMemoryStats"];
export type ServiceHealth = Schemas["ServiceHealthInfo"];
export type DatabaseHealth = Schemas["DatabaseHealthInfo"];
export type StorageHealth = Schemas["StorageHealthInfo"];
export type PoolStats = Schemas["DatabasePoolStats"];
export type SystemServicesHealth = Schemas["SystemServicesHealth"];
export type EnvironmentInfo = Schemas["SystemEnvironmentInfo"];
export type ServiceHealthStatus = ServiceHealth["status"];
export type SystemStatus = SystemInfo["status"];
