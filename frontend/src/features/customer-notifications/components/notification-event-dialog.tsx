"use client";

import { ArrowDown, ArrowUp, Eye, RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";
import { Controller, useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  EVENT_HINT_KEYS,
  MAX_BODY,
  MAX_REMINDER_OFFSETS,
  MAX_SUBJECT,
  NOTIFICATION_CHANNELS,
  REMINDER_OFFSET_PRESETS,
  VARIABLE_LABEL_KEYS,
  channelLabel,
  eventLabel,
  offsetLabel,
} from "@/features/customer-notifications/constants";
import {
  usePreviewCustomerNotification,
  useSaveCustomerNotification,
} from "@/features/customer-notifications/hooks/use-customer-notifications";
import type {
  CustomerNotificationEvent,
  CustomerNotificationPreview,
  CustomerNotificationTemplate,
  NotificationChannelName,
} from "@/features/customer-notifications/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "customer-notification-event-form";

type TemplateValues = {
  channel: NotificationChannelName;
  subject: string;
  body: string;
};

type EventFormValues = {
  enabled: boolean;
  fallback: boolean;
  channels: NotificationChannelName[];
  reminder_offsets: string[];
  templates: TemplateValues[];
};

/**
 * What the text editor needs of an event: its code (for the preview), its
 * variables and its texts. The confirmation request (TEC-979) passes its
 * own texts.
 */
export type TemplateSource = {
  code: string;
  variables: string[];
  templates: CustomerNotificationTemplate[];
};

function templateFor(
  event: TemplateSource,
  channel: NotificationChannelName,
): CustomerNotificationTemplate {
  return (
    event.templates.find((tpl) => tpl.channel === channel) ?? {
      channel,
      subject: "",
      body: "",
      custom: false,
      default_subject: "",
      default_body: "",
    }
  );
}

/**
 * One event's rule and texts: on / off, the ordered channels (the next one
 * is tried when one fails or has no ready provider, with fallback on),
 * reminder times and a text per channel with the variables and a preview.
 * An empty text, or the default one, goes back to the default.
 */
export function NotificationEventDialog({
  event,
  readiness,
  readOnly,
  onClose,
}: {
  event: CustomerNotificationEvent | null;
  readiness: Record<string, boolean>;
  readOnly: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const save = useSaveCustomerNotification();

  const schema = useMemo(() => {
    const reminder = Boolean(event?.reminder);
    return z
      .object({
        enabled: z.boolean(),
        fallback: z.boolean(),
        channels: z.array(z.enum(["sms", "whatsapp", "email"])),
        reminder_offsets: z.array(z.string()),
        templates: z.array(
          z.object({
            channel: z.enum(["sms", "whatsapp", "email"]),
            subject: z
              .string()
              .max(
                MAX_SUBJECT,
                t("customer_notifications.validation.subject_too_long"),
              ),
            body: z
              .string()
              .max(
                MAX_BODY,
                t("customer_notifications.validation.body_too_long"),
              ),
          }),
        ),
      })
      .superRefine((values, ctx) => {
        if (values.enabled && values.channels.length === 0) {
          ctx.addIssue({
            code: "custom",
            path: ["channels"],
            message: t("customer_notifications.validation.channels_required"),
          });
        }
        if (reminder && values.enabled && !values.reminder_offsets.length) {
          ctx.addIssue({
            code: "custom",
            path: ["reminder_offsets"],
            message: t("customer_notifications.validation.offsets_required"),
          });
        }
        if (reminder && values.reminder_offsets.length > MAX_REMINDER_OFFSETS) {
          ctx.addIssue({
            code: "custom",
            path: ["reminder_offsets"],
            message: t("customer_notifications.validation.offsets_too_many", {
              count: MAX_REMINDER_OFFSETS,
            }),
          });
        }
        values.templates.forEach((tpl, index) => {
          if (
            tpl.channel === "email" &&
            tpl.body.trim() &&
            !tpl.subject.trim()
          ) {
            ctx.addIssue({
              code: "custom",
              path: ["templates", index, "subject"],
              message: t("customer_notifications.validation.subject_required"),
            });
          }
        });
      });
  }, [event?.reminder, t]);

  const defaultValues = useMemo<EventFormValues | undefined>(() => {
    if (!event) return undefined;
    return {
      enabled: event.rule.enabled,
      fallback: event.rule.fallback,
      channels: event.rule.channels,
      reminder_offsets: event.rule.reminder_offsets.map(String),
      templates: NOTIFICATION_CHANNELS.map((channel) => {
        const tpl = templateFor(event, channel);
        return { channel, subject: tpl.subject, body: tpl.body };
      }),
    };
  }, [event]);

  return (
    <AppDialog
      open={Boolean(event)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={event ? eventLabel(t, event.code) : ""}
      description={event ? t(EVENT_HINT_KEYS[event.code]) : undefined}
      className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {readOnly ? t("common.close") : t("form.cancel")}
          </Button>
          {readOnly ? null : (
            <Button type="submit" form={FORM_ID} disabled={save.isPending}>
              {t("customer_notifications.editor.save")}
            </Button>
          )}
        </>
      }
    >
      {event && defaultValues ? (
        <AppForm<EventFormValues>
          key={event.code}
          id={FORM_ID}
          schema={schema}
          defaultValues={defaultValues}
          onSubmit={async (values) => {
            await save.mutateAsync({
              event: event.code,
              enabled: values.enabled,
              fallback: values.fallback,
              channels: values.channels,
              reminder_offsets: event.reminder
                ? values.reminder_offsets.map(Number)
                : undefined,
              templates: values.templates.map((tpl) => ({
                channel: tpl.channel,
                body: tpl.body.trim(),
                ...(tpl.channel === "email"
                  ? { subject: tpl.subject.trim() }
                  : {}),
              })),
            });
            onClose();
          }}
        >
          <div className="space-y-5">
            <AppSwitch
              name="enabled"
              label={t("customer_notifications.editor.enabled")}
              description={t("customer_notifications.editor.enabled_hint")}
              disabled={readOnly}
            />
            <ChannelOrderField readiness={readiness} disabled={readOnly} />
            <AppSwitch
              name="fallback"
              label={t("customer_notifications.editor.fallback")}
              description={t("customer_notifications.editor.fallback_hint")}
              disabled={readOnly}
            />
            {event.reminder ? (
              <ReminderOffsetsField event={event} disabled={readOnly} />
            ) : null}
            <TemplatesField event={event} disabled={readOnly} />
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/**
 * Ordered channel choice: checked channels first, in sending order. Binds
 * the form's `channels` field; the confirmation settings use it too.
 */
export function ChannelOrderField({
  readiness,
  disabled,
  channels = NOTIFICATION_CHANNELS,
}: {
  readiness: Record<string, boolean>;
  disabled: boolean;
  /** Channels offered (default: the message channels). */
  channels?: readonly string[];
}) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<EventFormValues>();
  const error = errors.channels?.message;

  return (
    <Controller
      control={control}
      name="channels"
      render={({ field }) => {
        const selected: string[] = field.value ?? [];
        const rest = channels.filter((channel) => !selected.includes(channel));
        const move = (index: number, delta: number) => {
          const next = [...selected];
          const target = index + delta;
          if (target < 0 || target >= next.length) return;
          [next[index], next[target]] = [next[target], next[index]];
          field.onChange(next);
        };
        return (
          <div className="space-y-2">
            <div>
              <Label>{t("customer_notifications.editor.channels")}</Label>
              <p className="text-muted-foreground text-xs">
                {t("customer_notifications.editor.channels_hint")}
              </p>
            </div>
            <div
              className={cn(
                "divide-y rounded-md border",
                error && "border-destructive",
              )}
            >
              {[...selected, ...rest].map((channel) => {
                const index = selected.indexOf(channel);
                const on = index >= 0;
                const ready = Boolean(readiness[channel]);
                const id = `customer-notification-channel-${channel}`;
                return (
                  <div
                    key={channel}
                    className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2"
                  >
                    <Checkbox
                      id={id}
                      checked={on}
                      disabled={disabled}
                      onCheckedChange={(checked) =>
                        field.onChange(
                          checked
                            ? [...selected, channel]
                            : selected.filter((c) => c !== channel),
                        )
                      }
                    />
                    <Label htmlFor={id} className="min-w-0 flex-1">
                      {on ? (
                        <span className="text-muted-foreground tabular-nums">
                          {index + 1}.{" "}
                        </span>
                      ) : null}
                      {channelLabel(t, channel)}
                    </Label>
                    <StatusChip
                      label={t(
                        ready
                          ? "customer_notifications.readiness.ready"
                          : "customer_notifications.readiness.not_ready",
                      )}
                      tone={ready ? "success" : "warning"}
                    />
                    <div className="flex items-center">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="size-8"
                        disabled={disabled || !on || index === 0}
                        aria-label={t("customer_notifications.editor.move_up")}
                        onClick={() => move(index, -1)}
                      >
                        <ArrowUp className="size-4" />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="size-8"
                        disabled={
                          disabled || !on || index === selected.length - 1
                        }
                        aria-label={t(
                          "customer_notifications.editor.move_down",
                        )}
                        onClick={() => move(index, 1)}
                      >
                        <ArrowDown className="size-4" />
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
            {error ? (
              <p className="text-destructive text-sm">{String(error)}</p>
            ) : null}
          </div>
        );
      }}
    />
  );
}

function ReminderOffsetsField({
  event,
  disabled,
}: {
  event: CustomerNotificationEvent;
  disabled: boolean;
}) {
  const { t } = useLocale();
  const options = useMemo(() => {
    const values = new Set<number>(REMINDER_OFFSET_PRESETS);
    event.rule.reminder_offsets.forEach((value) => values.add(value));
    return [...values]
      .sort((a, b) => a - b)
      .map((value) => ({
        value: String(value),
        label: offsetLabel(t, value),
      }));
  }, [event.rule.reminder_offsets, t]);
  return (
    <AppMultiCombobox
      name="reminder_offsets"
      label={t("customer_notifications.editor.offsets")}
      description={t("customer_notifications.editor.offsets_hint", {
        count: MAX_REMINDER_OFFSETS,
      })}
      options={options}
      placeholder={t("customer_notifications.editor.offsets_placeholder")}
      disabled={disabled}
    />
  );
}

/**
 * One text per channel; the tab shows a dot when its text has an error.
 * Binds the form's `templates` field, one entry per channel in the order
 * of `channels`; the confirmation request texts use it too.
 */
export function TemplatesField({
  event,
  disabled,
  channels = NOTIFICATION_CHANNELS,
  hint,
}: {
  event: TemplateSource;
  disabled: boolean;
  /** Channels with a text (default: the message channels). */
  channels?: readonly NotificationChannelName[];
  /** Replaces the default hint under the title. */
  hint?: string;
}) {
  const { t } = useLocale();
  const {
    formState: { errors },
  } = useFormContext<EventFormValues>();
  const [tab, setTab] = useState<string>(channels[0]);

  return (
    <div className="space-y-3">
      <div>
        <Label>{t("customer_notifications.editor.texts")}</Label>
        <p className="text-muted-foreground text-xs">
          {hint ?? t("customer_notifications.editor.texts_hint")}
        </p>
      </div>
      <Tabs value={tab} onValueChange={setTab} className="space-y-3">
        <TabsList>
          {channels.map((channel, index) => (
            <TabsTrigger key={channel} value={channel}>
              {channelLabel(t, channel)}
              {errors.templates?.[index] ? (
                <span
                  className="bg-destructive ml-1.5 inline-block size-1.5 rounded-full"
                  aria-hidden
                />
              ) : null}
            </TabsTrigger>
          ))}
        </TabsList>
        {channels.map((channel, index) => (
          <TabsContent
            key={channel}
            value={channel}
            forceMount
            className="data-[state=inactive]:hidden"
          >
            <TemplateEditor
              event={event}
              index={index}
              channel={channel}
              disabled={disabled}
            />
          </TabsContent>
        ))}
      </Tabs>
      <VariablesList variables={event.variables} />
    </div>
  );
}

function TemplateEditor({
  event,
  index,
  channel,
  disabled,
}: {
  event: TemplateSource;
  index: number;
  channel: NotificationChannelName;
  disabled: boolean;
}) {
  const { t } = useLocale();
  const { control, setValue, getValues } = useFormContext<EventFormValues>();
  const preview = usePreviewCustomerNotification();
  const [result, setResult] = useState<CustomerNotificationPreview | null>(
    null,
  );
  const tpl = templateFor(event, channel);
  const subjectName = `templates.${index}.subject` as const;
  const bodyName = `templates.${index}.body` as const;
  const body = useWatch({ control, name: bodyName }) ?? "";
  const subject = useWatch({ control, name: subjectName }) ?? "";
  const isEmail = channel === "email";
  const isDefault =
    body.trim() === tpl.default_body.trim() &&
    (!isEmail || subject.trim() === tpl.default_subject.trim());

  const restore = () => {
    setValue(bodyName, tpl.default_body, { shouldDirty: true });
    if (isEmail) {
      setValue(subjectName, tpl.default_subject, { shouldDirty: true });
    }
    setResult(null);
  };

  const runPreview = async () => {
    const values = getValues(`templates.${index}`);
    const next = await preview.mutateAsync({
      event: event.code,
      channel,
      subject: isEmail ? values.subject : undefined,
      body: values.body.trim() ? values.body : tpl.default_body,
    });
    setResult(next);
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={isDefault ? "secondary" : "outline"}>
          {t(
            isDefault
              ? "customer_notifications.editor.default_text"
              : "customer_notifications.editor.custom_text",
          )}
        </Badge>
        <div className="ml-auto flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={preview.isPending}
            onClick={() => void runPreview()}
          >
            <Eye className="size-4" />
            {t("customer_notifications.editor.preview")}
          </Button>
          {disabled ? null : (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={isDefault}
              onClick={restore}
            >
              <RotateCcw className="size-4" />
              {t("customer_notifications.editor.restore_default")}
            </Button>
          )}
        </div>
      </div>
      {isEmail ? (
        <AppInput
          name={subjectName}
          label={t("customer_notifications.editor.subject")}
          maxLength={MAX_SUBJECT}
          disabled={disabled}
        />
      ) : null}
      <AppTextarea
        name={bodyName}
        label={t("customer_notifications.editor.body")}
        description={t("customer_notifications.editor.body_count", {
          count: body.length,
          max: MAX_BODY,
        })}
        rows={isEmail ? 9 : 4}
        maxLength={MAX_BODY}
        disabled={disabled}
      />
      {channel === "whatsapp" ? (
        <p className="text-muted-foreground text-xs">
          {t("customer_notifications.editor.whatsapp_note")}
        </p>
      ) : null}
      {result ? (
        <div className="bg-muted/40 space-y-2 rounded-md border p-3 text-sm">
          <p className="text-muted-foreground text-xs font-medium">
            {t("customer_notifications.editor.preview_title")}
          </p>
          {isEmail && result.subject ? (
            <p className="font-medium break-words">{result.subject}</p>
          ) : null}
          <p className="break-words whitespace-pre-wrap">{result.body}</p>
          {result.unknown.length ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t("customer_notifications.editor.unknown_variables", {
                  names: result.unknown.join(", "),
                })}
              </AlertDescription>
            </Alert>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function VariablesList({ variables }: { variables: string[] }) {
  const { t } = useLocale();
  return (
    <div className="space-y-2">
      <p className="text-muted-foreground text-xs">
        {t("customer_notifications.editor.variables_hint")}
      </p>
      <div className="grid gap-1.5 sm:grid-cols-2">
        {variables.map((name) => (
          <div key={name} className="flex min-w-0 items-center gap-2 text-sm">
            <Badge variant="outline" className="shrink-0 font-mono">
              {`{{${name}}}`}
            </Badge>
            <span className="text-muted-foreground truncate">
              {VARIABLE_LABEL_KEYS[name] ? t(VARIABLE_LABEL_KEYS[name]) : name}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
