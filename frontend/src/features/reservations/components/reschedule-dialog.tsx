"use client";

import { useEffect, useMemo, useRef } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSelect, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { SlotPicker } from "@/features/reservations/components/slot-picker";
import {
  useReservation,
  useRescheduleReservation,
} from "@/features/reservations/hooks/use-reservations";
import { useSlotSearch } from "@/features/reservations/hooks/use-slot-search";
import { staffOptions } from "@/features/reservations/lib/staff-options";
import {
  ANY_STAFF,
  createRescheduleSchema,
  toLineInputs,
  toPartySize,
  type RescheduleFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import type { Reservation } from "@/features/reservations/types";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { dateKeyInZone, datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "reservation-reschedule-form";

/**
 * Moves a request or a booking to another free start (same services; the
 * person of each line may change). The old time is let go only when the new
 * one is taken, in the same step on the API.
 */
export function RescheduleDialog({
  uuid,
  onOpenChange,
}: {
  uuid: string | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const query = useReservation(uuid);
  const reservation = query.data;

  return (
    <AppDialog
      open={Boolean(uuid)}
      onOpenChange={onOpenChange}
      className="sm:max-w-2xl"
      title={
        reservation
          ? `${t("reservations.actions.reschedule")} · #${reservation.number}`
          : t("reservations.actions.reschedule")
      }
      description={t("reservations.reschedule.description")}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            disabled={
              !reservation ||
              !reservation.lines.every((line) => line.service_uuid)
            }
          >
            {t("reservations.actions.reschedule")}
          </Button>
        </>
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {reservation ? (
        <RescheduleForm
          key={`${reservation.uuid}-${reservation.version}`}
          reservation={reservation}
          onDone={() => onOpenChange(false)}
        />
      ) : null}
    </AppDialog>
  );
}

function RescheduleForm({
  reservation,
  onDone,
}: {
  reservation: Reservation;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const zone = organization?.timezone ?? undefined;
  const reschedule = useRescheduleReservation();
  // Restaurants may change the party with the time (a bigger table).
  const party = Boolean(useSector().sector?.capacity.party_size);
  const schema = useMemo(
    () => createRescheduleSchema(t, { party }),
    [t, party],
  );
  // A deleted service cannot be planned again.
  const editableLines = reservation.lines.every((line) => line.service_uuid);

  return (
    <AppForm<RescheduleFormValues>
      id={FORM_ID}
      className="grid gap-4"
      schema={schema}
      defaultValues={{
        lines: reservation.lines.map((line) => ({
          service_uuid: line.service_uuid ?? "",
          staff_uuid: line.staff?.uuid ?? ANY_STAFF,
        })),
        party_size:
          reservation.party_size != null ? String(reservation.party_size) : "",
        date: dateKeyInZone(reservation.starts_at, zone),
        starts_at: "",
        note: "",
      }}
      onSubmit={async (values) => {
        await reschedule.mutateAsync({
          uuid: reservation.uuid,
          body: {
            starts_at: values.starts_at,
            lines: toLineInputs(values.lines),
            ...(party ? { party_size: toPartySize(values.party_size) } : {}),
            expected_version: reservation.version,
            note: values.note || undefined,
          },
        });
        onDone();
      }}
    >
      <p className="text-muted-foreground text-sm">
        {t("reservations.reschedule.current")}:{" "}
        <span className="text-foreground tabular-nums">
          {datetimeInZone(reservation.starts_at, zone, undefined, {
            dateStyle: "medium",
            timeStyle: "short",
          })}
        </span>
      </p>
      <RescheduleFields
        reservation={reservation}
        editableLines={editableLines}
        party={party}
      />
      <AppTextarea
        name="note"
        label={t("reservations.fields.note")}
        description={t("reservations.fields.note_hint")}
      />
    </AppForm>
  );
}

function RescheduleFields({
  reservation,
  editableLines,
  party,
}: {
  reservation: Reservation;
  editableLines: boolean;
  party: boolean;
}) {
  const { t } = useLocale();
  const { control, setValue, formState } =
    useFormContext<RescheduleFormValues>();
  const [lines, date, startsAt, partyInput] = useWatch({
    control,
    name: ["lines", "date", "starts_at", "party_size"],
  });
  const partySize = toPartySize(partyInput);
  // The booked area stays; the search keeps to it.
  const { query, ready } = useSlotSearch({
    branchUuid: editableLines ? reservation.branch.uuid : "",
    lines,
    date,
    partySize,
    zone: reservation.zone || undefined,
    partyRequired: party,
  });

  // Another person, party or day: the picked start belongs to the old answer.
  const inputs = `${JSON.stringify(toLineInputs(lines))}|${date}|${partySize ?? ""}`;
  const lastInputs = useRef(inputs);
  useEffect(() => {
    if (lastInputs.current === inputs) return;
    lastInputs.current = inputs;
    setValue("starts_at", "");
  }, [inputs, setValue]);

  if (!editableLines) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("reservations.reschedule.service_deleted")}
      </p>
    );
  }

  return (
    <>
      {party ? (
        <AppInput
          name="party_size"
          label={t("reservations.fields.party_size")}
          description={
            reservation.zone
              ? t("reservations.fields.party_size_zone_hint", {
                  zone: reservation.zone,
                })
              : t("reservations.fields.party_size_hint")
          }
          inputMode="numeric"
          autoComplete="off"
        />
      ) : null}
      {reservation.lines.map((line, index) => (
        <div
          key={line.index}
          className="grid items-end gap-3 sm:grid-cols-[1fr_1fr]"
        >
          <p className="pb-2 text-sm font-medium">{line.service_name}</p>
          <AppSelect
            name={`lines.${index}.staff_uuid`}
            label={t("reservations.fields.staff")}
            options={withCurrent(
              staffOptions(t, query.data, index),
              line.staff,
            )}
          />
        </div>
      ))}
      <SlotPicker
        query={query}
        ready={ready}
        date={date}
        onDateChange={(next) =>
          setValue("date", next, { shouldValidate: true })
        }
        value={startsAt}
        onChange={(slot) =>
          setValue("starts_at", slot?.starts_at ?? "", {
            shouldValidate: true,
          })
        }
        error={formState.errors.starts_at?.message}
      />
    </>
  );
}

/** Keeps the booked person selectable before the API's list arrives. */
function withCurrent(
  options: { value: string; label: string }[],
  staff: Reservation["lines"][number]["staff"],
) {
  if (!staff?.uuid || options.some((option) => option.value === staff.uuid)) {
    return options;
  }
  return [...options, { value: staff.uuid, label: staff.name }];
}
