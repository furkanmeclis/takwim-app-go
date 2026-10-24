"use client";

import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSelect, AppSwitch } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  CALL_RETRY_PRESETS,
  DEADLINE_PRESETS,
  FIRST_REQUEST_PRESETS,
  FOLLOW_UP_PRESETS,
  ON_NO_ANSWER_HINT_KEYS,
  ON_NO_ANSWER_KEYS,
  clock,
  deadlineLabel,
  followUpLabel,
  minuteOptions,
} from "@/features/confirmations/constants";
import { useSaveConfirmationRule } from "@/features/confirmations/hooks/use-confirmations";
import type {
  ConfirmationLimits,
  ConfirmationRuleTarget,
  OnNoAnswer,
} from "@/features/confirmations/types";
import { ChannelOrderField } from "@/features/customer-notifications/components/notification-event-dialog";
import { offsetLabel } from "@/features/customer-notifications/constants";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "confirmation-rule-form";
const CHANNELS = ["sms", "whatsapp", "email", "voice_call"] as const;
type RuleChannel = (typeof CHANNELS)[number];

/** Field names match the API so its field errors land on the inputs. */
type RuleFormValues = {
  enabled: boolean;
  channels: RuleChannel[];
  fallback: boolean;
  first_request_minutes: string;
  follow_up_minutes: string;
  max_attempts: string;
  deadline_minutes: string;
  window: boolean;
  window_start_minute: string;
  window_end_minute: string;
  on_no_answer: OnNoAnswer;
  call_window: boolean;
  call_window_start_minute: string;
  call_window_end_minute: string;
  call_retries: string;
  call_retry_minutes: string;
};

/**
 * One confirmation rule: on / off, the channels in order, when the first
 * request and the follow-up go out, how many requests, the last answer,
 * the hours requests may go out, the call hours and extra calls (when the
 * automatic call is a channel) and what happens without an answer.
 */
export function ConfirmationRuleDialog({
  target,
  limits,
  readiness,
  readOnly,
  onClose,
}: {
  target: ConfirmationRuleTarget | null;
  limits: ConfirmationLimits | undefined;
  readiness: Record<string, boolean>;
  readOnly: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const save = useSaveConfirmationRule();
  const response = limits?.min_response_minutes ?? 30;
  const minWindow = limits?.min_window_minutes ?? 60;

  const schema = useMemo(
    () =>
      z
        .object({
          enabled: z.boolean(),
          channels: z.array(z.enum(CHANNELS)),
          fallback: z.boolean(),
          first_request_minutes: z.string(),
          follow_up_minutes: z.string(),
          max_attempts: z.string(),
          deadline_minutes: z.string(),
          window: z.boolean(),
          window_start_minute: z.string(),
          window_end_minute: z.string(),
          on_no_answer: z.enum(["cancel", "flag"]),
          call_window: z.boolean(),
          call_window_start_minute: z.string(),
          call_window_end_minute: z.string(),
          call_retries: z.string(),
          call_retry_minutes: z.string(),
        })
        .superRefine((v, ctx) => {
          const first = Number(v.first_request_minutes);
          const follow = Number(v.follow_up_minutes);
          const deadline = Number(v.deadline_minutes);
          const attempts = Number(v.max_attempts);
          const issue = (path: string, key: string) =>
            ctx.addIssue({
              code: "custom",
              path: [path],
              message: t(key, { minutes: response, window: minWindow }),
            });
          if (!v.channels.length) {
            issue("channels", "confirmations.validation.channels_required");
          }
          if (first - deadline < response) {
            issue(
              "deadline_minutes",
              "confirmations.validation.deadline_after_first",
            );
          }
          if (follow > 0 && follow >= first) {
            issue(
              "follow_up_minutes",
              "confirmations.validation.follow_up_after_first",
            );
          } else if (follow > 0 && follow - deadline < response) {
            issue(
              "follow_up_minutes",
              "confirmations.validation.follow_up_before_deadline",
            );
          }
          if (attempts > 1 && follow === 0) {
            issue("max_attempts", "confirmations.validation.needs_follow_up");
          }
          if (
            v.window &&
            Number(v.window_end_minute) - Number(v.window_start_minute) <
              minWindow
          ) {
            issue(
              "window_end_minute",
              "confirmations.validation.window_too_short",
            );
          }
          if (v.channels.includes("voice_call") && v.call_window) {
            const start = Number(v.call_window_start_minute);
            const end = Number(v.call_window_end_minute);
            if (end - start < minWindow) {
              issue(
                "call_window_end_minute",
                "confirmations.validation.window_too_short",
              );
            } else if (
              v.window &&
              Math.min(end, Number(v.window_end_minute)) -
                Math.max(start, Number(v.window_start_minute)) <
                minWindow
            ) {
              issue(
                "call_window_end_minute",
                "confirmations.validation.call_window_overlap",
              );
            }
          }
        }),
    [minWindow, response, t],
  );

  const defaultValues = useMemo<RuleFormValues | undefined>(() => {
    if (!target) return undefined;
    const r = target.rule;
    const hasWindow =
      r.window_start_minute != null && r.window_end_minute != null;
    const hasCallWindow =
      r.call_window_start_minute != null && r.call_window_end_minute != null;
    return {
      enabled: r.enabled,
      channels: r.channels,
      fallback: r.fallback,
      first_request_minutes: String(r.first_request_minutes),
      follow_up_minutes: String(r.follow_up_minutes),
      max_attempts: String(r.max_attempts),
      deadline_minutes: String(r.deadline_minutes),
      window: hasWindow,
      window_start_minute: String(r.window_start_minute ?? 9 * 60),
      window_end_minute: String(r.window_end_minute ?? 21 * 60),
      on_no_answer: r.on_no_answer,
      call_window: hasCallWindow,
      call_window_start_minute: String(r.call_window_start_minute ?? 10 * 60),
      call_window_end_minute: String(r.call_window_end_minute ?? 20 * 60),
      call_retries: String(r.call_retries),
      call_retry_minutes: String(r.call_retry_minutes || 30),
    };
  }, [target]);

  const title = target
    ? target.branch
      ? t("confirmations.editor.title_branch", { branch: target.branch.name })
      : t("confirmations.editor.title_default")
    : "";

  return (
    <AppDialog
      open={Boolean(target)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={title}
      description={t("confirmations.editor.description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {readOnly ? t("common.close") : t("form.cancel")}
          </Button>
          {readOnly ? null : (
            <Button type="submit" form={FORM_ID} disabled={save.isPending}>
              {t("confirmations.editor.save")}
            </Button>
          )}
        </>
      }
    >
      {target && defaultValues ? (
        <AppForm<RuleFormValues>
          key={target.branch?.uuid ?? "default"}
          id={FORM_ID}
          schema={schema}
          defaultValues={defaultValues}
          onSubmit={async (v) => {
            await save.mutateAsync({
              branch_uuid: target.branch?.uuid ?? null,
              enabled: v.enabled,
              channels: v.channels,
              fallback: v.fallback,
              first_request_minutes: Number(v.first_request_minutes),
              follow_up_minutes: Number(v.follow_up_minutes),
              max_attempts: Number(v.max_attempts),
              deadline_minutes: Number(v.deadline_minutes),
              window_start_minute: v.window
                ? Number(v.window_start_minute)
                : null,
              window_end_minute: v.window ? Number(v.window_end_minute) : null,
              on_no_answer: v.on_no_answer,
              call_window_start_minute: v.call_window
                ? Number(v.call_window_start_minute)
                : null,
              call_window_end_minute: v.call_window
                ? Number(v.call_window_end_minute)
                : null,
              call_retries: Number(v.call_retries),
              call_retry_minutes: Number(v.call_retry_minutes),
            });
            onClose();
          }}
        >
          <div className="space-y-5">
            {target.branch && !target.custom && !readOnly ? (
              <Alert>
                <AlertDescription>
                  {t("confirmations.editor.branch_follows_default")}
                </AlertDescription>
              </Alert>
            ) : null}
            <AppSwitch
              name="enabled"
              label={t("confirmations.editor.enabled")}
              description={t("confirmations.editor.enabled_hint")}
              disabled={readOnly}
            />
            <ChannelOrderField
              readiness={readiness}
              disabled={readOnly}
              channels={CHANNELS}
            />
            <AppSwitch
              name="fallback"
              label={t("confirmations.editor.fallback")}
              description={t("confirmations.editor.fallback_hint")}
              disabled={readOnly}
            />
            <TimingFields rule={target.rule} disabled={readOnly} />
            <WindowFields disabled={readOnly} />
            <CallFields disabled={readOnly} />
            <OnNoAnswerField disabled={readOnly} />
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

function TimingFields({
  rule,
  disabled,
}: {
  rule: ConfirmationRuleTarget["rule"];
  disabled: boolean;
}) {
  const { t } = useLocale();
  const attempts = useMemo(
    () =>
      [1, 2, 3, 4, 5].map((n) => ({
        value: String(n),
        label: t("confirmations.timing.attempts_value", { count: n }),
      })),
    [t],
  );
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <AppSelect
        name="first_request_minutes"
        label={t("confirmations.editor.first_request")}
        description={t("confirmations.editor.first_request_hint")}
        options={minuteOptions(
          FIRST_REQUEST_PRESETS,
          rule.first_request_minutes,
          (m) => offsetLabel(t, m),
        )}
        disabled={disabled}
      />
      <AppSelect
        name="follow_up_minutes"
        label={t("confirmations.editor.follow_up")}
        description={t("confirmations.editor.follow_up_hint")}
        options={minuteOptions(FOLLOW_UP_PRESETS, rule.follow_up_minutes, (m) =>
          followUpLabel(t, m),
        )}
        disabled={disabled}
      />
      <AppSelect
        name="max_attempts"
        label={t("confirmations.editor.max_attempts")}
        description={t("confirmations.editor.max_attempts_hint")}
        options={attempts}
        disabled={disabled}
      />
      <AppSelect
        name="deadline_minutes"
        label={t("confirmations.editor.deadline")}
        description={t("confirmations.editor.deadline_hint")}
        options={minuteOptions(DEADLINE_PRESETS, rule.deadline_minutes, (m) =>
          deadlineLabel(t, m),
        )}
        disabled={disabled}
      />
    </div>
  );
}

function WindowFields({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const { control } = useFormContext<RuleFormValues>();
  const on = useWatch({ control, name: "window" });
  const starts = useMemo(
    () =>
      Array.from({ length: 24 }, (_, h) => ({
        value: String(h * 60),
        label: clock(h * 60),
      })),
    [],
  );
  const ends = useMemo(
    () =>
      Array.from({ length: 24 }, (_, h) => ({
        value: String((h + 1) * 60),
        label: clock((h + 1) * 60),
      })),
    [],
  );
  return (
    <div className="space-y-3">
      <AppSwitch
        name="window"
        label={t("confirmations.editor.window")}
        description={t("confirmations.editor.window_hint")}
        disabled={disabled}
      />
      {on ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <AppSelect
            name="window_start_minute"
            label={t("confirmations.editor.window_start")}
            options={starts}
            disabled={disabled}
          />
          <AppSelect
            name="window_end_minute"
            label={t("confirmations.editor.window_end")}
            options={ends}
            disabled={disabled}
          />
        </div>
      ) : null}
    </div>
  );
}

/** Hours of the day as select options ("09:00"); end = the hour's end. */
function hourOptions(end: boolean) {
  return Array.from({ length: 24 }, (_, h) => {
    const minute = (end ? h + 1 : h) * 60;
    return { value: String(minute), label: clock(minute) };
  });
}

/**
 * The automatic call (TEC-923): the hours it may ring and how often a call
 * without an answer is made again. Shown only when the call is a channel.
 */
function CallFields({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const { control } = useFormContext<RuleFormValues>();
  const channels = useWatch({ control, name: "channels" });
  const own = useWatch({ control, name: "call_window" });
  const retries = useWatch({ control, name: "call_retries" });
  const starts = useMemo(() => hourOptions(false), []);
  const ends = useMemo(() => hourOptions(true), []);
  const retryOptions = useMemo(
    () =>
      [0, 1, 2, 3].map((n) => ({
        value: String(n),
        label:
          n === 0
            ? t("confirmations.calls.no_retry")
            : t("confirmations.calls.retries_count", { count: n }),
      })),
    [t],
  );
  const waitOptions = useMemo(
    () =>
      CALL_RETRY_PRESETS.map((m) => ({
        value: String(m),
        label: t("confirmations.calls.wait_value", { minutes: m }),
      })),
    [t],
  );
  if (!channels?.includes("voice_call")) return null;
  return (
    <div className="space-y-3 rounded-md border p-3">
      <div>
        <p className="text-sm font-medium">{t("confirmations.calls.title")}</p>
        <p className="text-muted-foreground text-xs">
          {t("confirmations.calls.hint")}
        </p>
      </div>
      <AppSwitch
        name="call_window"
        label={t("confirmations.calls.own_window")}
        description={t("confirmations.calls.own_window_hint")}
        disabled={disabled}
      />
      {own ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <AppSelect
            name="call_window_start_minute"
            label={t("confirmations.editor.window_start")}
            options={starts}
            disabled={disabled}
          />
          <AppSelect
            name="call_window_end_minute"
            label={t("confirmations.editor.window_end")}
            options={ends}
            disabled={disabled}
          />
        </div>
      ) : null}
      <div className="grid gap-4 sm:grid-cols-2">
        <AppSelect
          name="call_retries"
          label={t("confirmations.calls.retries")}
          description={t("confirmations.calls.retries_hint")}
          options={retryOptions}
          disabled={disabled}
        />
        {retries !== "0" ? (
          <AppSelect
            name="call_retry_minutes"
            label={t("confirmations.calls.wait")}
            options={waitOptions}
            disabled={disabled}
          />
        ) : null}
      </div>
    </div>
  );
}

function OnNoAnswerField({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const { control } = useFormContext<RuleFormValues>();
  const value = useWatch({ control, name: "on_no_answer" });
  const options = (Object.keys(ON_NO_ANSWER_KEYS) as OnNoAnswer[]).map(
    (key) => ({ value: key, label: t(ON_NO_ANSWER_KEYS[key]) }),
  );
  return (
    <AppSelect
      name="on_no_answer"
      label={t("confirmations.editor.on_no_answer")}
      description={t(ON_NO_ANSWER_HINT_KEYS[value ?? "cancel"])}
      options={options}
      disabled={disabled}
    />
  );
}
