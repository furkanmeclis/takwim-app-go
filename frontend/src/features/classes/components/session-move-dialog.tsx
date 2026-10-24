"use client";

import { useMemo } from "react";
import { z } from "zod";

import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useBusinessToday } from "@/features/cash/lib/format";
import { useSessionWhen } from "@/features/classes/components/sessions-table";
import { MAX_SESSION_DAYS } from "@/features/classes/constants";
import {
  useClassSessions,
  useMoveClassSessionBookings,
} from "@/features/classes/hooks/use-classes";
import { addDays } from "@/features/classes/lib/filters";
import type { ClassSession } from "@/features/classes/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "class-session-move-form";

/** Open sessions of the class before the source are offered from this many days back. */
const LOOK_BACK_DAYS = 14;

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function createSchema(t: Translate) {
  return z.object({
    to_date: z.string().min(1, t("classes.validation.to_date")),
  });
}

type MoveFormValues = z.infer<ReturnType<typeof createSchema>>;

/**
 * Moves the open bookings of a session to another open session of the
 * same class (TEC-994). The target is picked among the class's open
 * sessions in the days around the source (today onwards, at most the
 * session list's window). Bookings that find no seat stay in the session;
 * the run can be undone from the session page within the bulk window.
 */
export function SessionMoveDialog({
  session,
  openBookings,
  open,
  onOpenChange,
}: {
  session: ClassSession;
  /** Requests and active bookings of the session. */
  openBookings: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const today = useBusinessToday();
  const when = useSessionWhen();
  const move = useMoveClassSessionBookings();
  const schema = useMemo(() => createSchema(t), [t]);

  // The window: from today (or two weeks before a far session) for the
  // longest period the session list serves.
  const from = useMemo(() => {
    const back = addDays(session.date, -LOOK_BACK_DAYS);
    return back > today ? back : today;
  }, [session.date, today]);
  const to = useMemo(() => addDays(from, MAX_SESSION_DAYS - 1), [from]);
  const sessions = useClassSessions({
    from,
    to,
    service: session.service.uuid,
    branch: session.branch.uuid,
    state: "open",
    limit: 100,
    offset: 0,
  });
  const options = useMemo(
    () =>
      (sessions.data?.items ?? [])
        .filter(
          (s) =>
            s.schedule_uuid === session.schedule_uuid &&
            s.date !== session.date,
        )
        .map((s) => {
          const w = when(s);
          return {
            value: s.date,
            label: `${w.day} · ${w.time} · ${t("classes.move_dialog.seats_left", { count: s.seats_left })}`,
          };
        }),
    [session.date, session.schedule_uuid, sessions.data, t, when],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("classes.move_dialog.title")}
      description={t("classes.move_dialog.description", {
        count: openBookings,
      })}
      className="sm:max-w-md"
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
            disabled={move.isPending || options.length === 0}
          >
            {t("classes.actions.move_bookings")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<MoveFormValues>
          key={`${session.schedule_uuid}-${session.date}`}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ to_date: "" }}
          onSubmit={async (values) => {
            await move.mutateAsync({
              uuid: session.schedule_uuid,
              date: session.date,
              toDate: values.to_date,
            });
            onOpenChange(false);
          }}
        >
          {sessions.isLoading ? (
            <Loading label={t("common.loading")} />
          ) : (
            <AppSelect
              name="to_date"
              label={t("classes.move_dialog.target")}
              placeholder={
                options.length === 0
                  ? t("classes.move_dialog.no_target")
                  : t("classes.move_dialog.target_placeholder")
              }
              description={t("classes.move_dialog.target_hint")}
              options={options}
              disabled={options.length === 0}
            />
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
