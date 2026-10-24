"use client";

import { Bike, Car, CarFront, CircleHelp, Truck } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import { MAX_PLATE, validPlate } from "@/features/business-page/lib/wizard";
import type { PublicVehicle } from "@/features/business-page/types";
import {
  VEHICLE_TYPES,
  VEHICLE_TYPE_KEYS,
} from "@/features/reservations/constants";
import { useLocale } from "@/providers/locale-provider";

const ICONS: Record<PublicVehicle["type"], LucideIcon> = {
  car: Car,
  suv: CarFront,
  van: Truck,
  motorcycle: Bike,
  other: CircleHelp,
};

/**
 * Vehicle step (car wash): the vehicle type as one tap and an optional
 * licence plate. The business sees both on the booking; the API stores the
 * plate upper case.
 */
export function VehicleStep({
  type,
  plate,
  onTypeChange,
  onPlateChange,
}: {
  type: PublicVehicle["type"] | "";
  plate: string;
  onTypeChange: (type: PublicVehicle["type"]) => void;
  onPlateChange: (plate: string) => void;
}) {
  const { t } = useLocale();
  const invalid = !validPlate(plate);

  return (
    <div className="space-y-5">
      <RadioGroup
        value={type}
        onValueChange={(next) => onTypeChange(next as PublicVehicle["type"])}
        className="grid gap-2 sm:grid-cols-2"
        aria-label={t("business_page.vehicle.type")}
      >
        {VEHICLE_TYPES.map((kind) => {
          const Icon = ICONS[kind];
          const id = `vehicle-${kind}`;
          return (
            <ChoiceCard
              key={kind}
              id={id}
              title={
                <span className="flex items-center gap-2">
                  <Icon className="text-muted-foreground size-4" />
                  {t(VEHICLE_TYPE_KEYS[kind])}
                </span>
              }
              control={<RadioGroupItem value={kind} id={id} />}
            />
          );
        })}
      </RadioGroup>

      <Field data-invalid={invalid ? "true" : undefined}>
        <FieldLabel htmlFor="vehicle-plate">
          {t("business_page.vehicle.plate")}
        </FieldLabel>
        <Input
          id="vehicle-plate"
          value={plate}
          onChange={(event) => onPlateChange(event.target.value)}
          maxLength={MAX_PLATE + 4}
          autoCapitalize="characters"
          autoComplete="off"
          spellCheck={false}
          placeholder="34 ABC 123"
          aria-invalid={invalid}
          className="uppercase"
        />
        <FieldDescription>
          {invalid
            ? t("business_page.vehicle.plate_invalid", { max: MAX_PLATE })
            : t("business_page.vehicle.plate_hint")}
        </FieldDescription>
      </Field>
    </div>
  );
}
