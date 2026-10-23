import type {
  SectorCode,
  SectorModule,
  SectorResourceKind,
  SectorStep,
} from "@/features/sectors/types";

/** Display order of the sectors (requirements K-008). */
export const SECTOR_CODES: SectorCode[] = [
  "hairdresser",
  "restaurant",
  "gym",
  "car_wash",
  "clinic",
  "beauty_salon",
];

export const SECTOR_LABEL_KEYS: Record<SectorCode, string> = {
  hairdresser: "sectors.names.hairdresser",
  restaurant: "sectors.names.restaurant",
  gym: "sectors.names.gym",
  car_wash: "sectors.names.car_wash",
  clinic: "sectors.names.clinic",
  beauty_salon: "sectors.names.beauty_salon",
};

export const SECTOR_MODULE_LABEL_KEYS: Record<SectorModule, string> = {
  reservations: "sectors.modules.reservations",
  calendar: "sectors.modules.calendar",
  customers: "sectors.modules.customers",
  services: "sectors.modules.services",
  packages: "sectors.modules.packages",
  products: "sectors.modules.products",
  staff: "sectors.modules.staff",
  staff_shares: "sectors.modules.staff_shares",
  resources: "sectors.modules.resources",
  classes: "sectors.modules.classes",
  vehicles: "sectors.modules.vehicles",
  cash: "sectors.modules.cash",
  finance: "sectors.modules.finance",
  reports: "sectors.modules.reports",
  confirmation: "sectors.modules.confirmation",
  booking_page: "sectors.modules.booking_page",
};

export const SECTOR_STEP_LABEL_KEYS: Record<SectorStep, string> = {
  branch: "sectors.steps.branch",
  service: "sectors.steps.service",
  staff: "sectors.steps.staff",
  party_size: "sectors.steps.party_size",
  session_type: "sectors.steps.session_type",
  class: "sectors.steps.class",
  vehicle: "sectors.steps.vehicle",
  appointment_type: "sectors.steps.appointment_type",
  datetime: "sectors.steps.datetime",
  area: "sectors.steps.area",
  products: "sectors.steps.products",
  contact: "sectors.steps.contact",
  summary: "sectors.steps.summary",
  payment: "sectors.steps.payment",
};

export const SECTOR_RESOURCE_LABEL_KEYS: Record<SectorResourceKind, string> = {
  chair: "sectors.resources.chair",
  table: "sectors.resources.table",
  area: "sectors.resources.area",
  room: "sectors.resources.room",
  device: "sectors.resources.device",
  bay: "sectors.resources.bay",
};

/** Term codes the API sends in `sector.terms` (`sectors.terms.<code>`). */
export const SECTOR_TERM_KEYS: Record<string, string> = {
  staff: "sectors.terms.staff",
  specialist: "sectors.terms.specialist",
  trainer: "sectors.terms.trainer",
  chair: "sectors.terms.chair",
  table: "sectors.terms.table",
  area: "sectors.terms.area",
  room: "sectors.terms.room",
  room_device: "sectors.terms.room_device",
  bay: "sectors.terms.bay",
  appointment: "sectors.terms.appointment",
  reservation: "sectors.terms.reservation",
};
