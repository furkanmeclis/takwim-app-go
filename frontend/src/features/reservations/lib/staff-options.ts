import { ANY_STAFF } from "@/features/reservations/schemas/reservation-forms";
import type { Availability } from "@/features/reservations/types";

/** Staff choices of one line: anyone free, or a person the API offers. */
export function staffOptions(
  t: (key: string) => string,
  availability: Availability | undefined,
  index: number,
) {
  const line = availability?.lines[index];
  return [
    { value: ANY_STAFF, label: t("reservations.fields.any_staff") },
    ...(line?.staff ?? []).map((staff) => ({
      value: staff.uuid,
      label: staff.name,
    })),
  ];
}
