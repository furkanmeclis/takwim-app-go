"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppPassword, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  labelOf,
  INTEGRATION_FIELD_LABEL_KEYS,
  INTEGRATION_PROVIDER_HINT_KEYS,
  INTEGRATION_PROVIDER_LABEL_KEYS,
  integrationWebhookPath,
} from "@/features/integrations/constants";
import {
  useConnectIntegration,
  useIntegrationsMeta,
  useUpdateIntegration,
} from "@/features/integrations/hooks/use-integrations";
import type {
  IntegrationAccount,
  IntegrationProvider,
  IntegrationProviderCode,
} from "@/features/integrations/types";
import { useLocale } from "@/providers/locale-provider";

const CONNECT_FORM = "integration-connect-form";
const EDIT_FORM = "integration-edit-form";

type Field = IntegrationProvider["fields"][number];

/** The development log provider's fields until the meta call answers. */
const FALLBACK_PROVIDER: IntegrationProviderCode = "log";
const FALLBACK_FIELDS: Field[] = [
  {
    key: "sender",
    label_key: "integrations.fields.sender",
    secret: false,
    required: false,
  },
];

function CredentialInput({
  field,
  placeholder,
  description,
}: {
  field: Field;
  placeholder?: string;
  description?: string;
}) {
  const { t } = useLocale();
  const label = labelOf(t, INTEGRATION_FIELD_LABEL_KEYS, field.key);
  return field.secret ? (
    <AppPassword
      name={`credentials.${field.key}`}
      label={label}
      placeholder={placeholder}
      description={description}
      autoComplete="off"
    />
  ) : (
    <AppInput
      name={`credentials.${field.key}`}
      label={label}
      placeholder={placeholder}
      description={description}
      autoComplete="off"
    />
  );
}

type ConnectValues = {
  display_name: string;
  sandbox: boolean;
  credentials: Record<string, string>;
};

/**
 * Connect a provider with the organization's own credentials (SMS,
 * WhatsApp, e-mail, payments; the list comes from the meta call). The
 * secrets are sent once and never shown again (the API answers with masked
 * hints).
 */
export function ConnectIntegrationDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const connect = useConnectIntegration();
  const meta = useIntegrationsMeta();
  const providers = meta.data?.providers ?? [];
  const [providerCode, setProviderCode] =
    useState<IntegrationProviderCode>(FALLBACK_PROVIDER);
  const provider = providers.find((p) => p.provider === providerCode);
  const fields = useMemo(
    () =>
      provider?.fields ??
      (providerCode === FALLBACK_PROVIDER ? FALLBACK_FIELDS : []),
    [provider, providerCode],
  );
  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("integrations.validation.display_name"))
          .max(120),
        sandbox: z.boolean(),
        credentials: z.object(
          Object.fromEntries(
            fields.map((f) => [
              f.key,
              f.required
                ? z
                    .string()
                    .trim()
                    .min(1, t("integrations.validation.required"))
                : z.string(),
            ]),
          ),
        ),
      }),
    [fields, t],
  );
  const providerOptions = (
    providers.length ? providers.map((p) => p.provider) : [FALLBACK_PROVIDER]
  ).map((code) => ({
    value: code,
    label: labelOf(t, INTEGRATION_PROVIDER_LABEL_KEYS, code),
  }));
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("integrations.connect_title")}
      description={t("integrations.connect_description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={CONNECT_FORM}
            disabled={connect.isPending}
          >
            {t("integrations.connect")}
          </Button>
        </>
      }
    >
      {open ? (
        <div className="space-y-4">
          <ProviderPicker
            value={providerCode}
            options={providerOptions}
            onChange={setProviderCode}
          />
          <AppForm<ConnectValues>
            key={providerCode}
            id={CONNECT_FORM}
            schema={schema}
            defaultValues={{
              display_name: labelOf(
                t,
                INTEGRATION_PROVIDER_LABEL_KEYS,
                providerCode,
              ),
              sandbox: true,
              credentials: Object.fromEntries(fields.map((f) => [f.key, ""])),
            }}
            onSubmit={async (values) => {
              const credentials: Record<string, string> = {};
              for (const f of fields) {
                const v = (values.credentials[f.key] ?? "").trim();
                if (v) credentials[f.key] = v;
              }
              await connect.mutateAsync({
                provider: providerCode,
                kind: provider?.kind,
                display_name: values.display_name.trim(),
                mode: values.sandbox ? "sandbox" : "live",
                credentials,
              });
              onClose();
            }}
          >
            <div className="space-y-4">
              <p className="text-muted-foreground text-sm">
                {labelOf(t, INTEGRATION_PROVIDER_HINT_KEYS, providerCode)}
              </p>
              <AppInput
                name="display_name"
                label={t("integrations.fields.display_name")}
              />
              <AppSwitch
                name="sandbox"
                label={t("integrations.fields.sandbox")}
                description={t("integrations.fields.sandbox_hint")}
              />
              {fields.map((f) => (
                <CredentialInput key={f.key} field={f} />
              ))}
              <p className="text-muted-foreground text-xs">
                {t("integrations.secrets_note")}
              </p>
            </div>
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}

/** Provider choice outside the credential form (the form remounts per provider). */
function ProviderPicker({
  value,
  options,
  onChange,
}: {
  value: IntegrationProviderCode;
  options: { value: string; label: string }[];
  onChange: (value: IntegrationProviderCode) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-2">
      <Label htmlFor="integration-provider">
        {t("integrations.fields.provider")}
      </Label>
      <Select
        value={value}
        onValueChange={(next) => onChange(next as IntegrationProviderCode)}
      >
        <SelectTrigger id="integration-provider" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

type EditValues = {
  display_name: string;
  sandbox: boolean;
  credentials: Record<string, string>;
};

/**
 * The callback URL the business enters at its provider (WhatsApp Cloud
 * API: Meta app → WhatsApp → Configuration), with the verify token of the
 * account.
 */
function WebhookAddress({ account }: { account: IntegrationAccount }) {
  const { t } = useLocale();
  const path = integrationWebhookPath(account);
  if (!path) return null;
  const url =
    typeof window === "undefined" ? path : `${window.location.origin}${path}`;
  return (
    <div className="space-y-1.5">
      <Label>{t("integrations.webhook.label")}</Label>
      <p className="bg-muted rounded-md px-3 py-2 font-mono text-xs break-all select-all">
        {url}
      </p>
      <p className="text-muted-foreground text-xs">
        {t("integrations.webhook.hint")}
      </p>
    </div>
  );
}

/** Rename, switch sandbox / live, rotate secrets (empty = keep). */
export function EditIntegrationDialog({
  account,
  onClose,
}: {
  account: IntegrationAccount | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const update = useUpdateIntegration();
  const meta = useIntegrationsMeta();
  const fields = useMemo(
    () =>
      meta.data?.providers.find((p) => p.provider === account?.provider)
        ?.fields ??
      (account?.provider === FALLBACK_PROVIDER ? FALLBACK_FIELDS : []),
    [account?.provider, meta.data],
  );
  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("integrations.validation.display_name"))
          .max(120),
        sandbox: z.boolean(),
        credentials: z.object(
          Object.fromEntries(fields.map((f) => [f.key, z.string()])),
        ),
      }),
    [fields, t],
  );
  return (
    <AppDialog
      open={Boolean(account)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("integrations.edit_title")}
      description={t("integrations.edit_description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={EDIT_FORM} disabled={update.isPending}>
            {t("integrations.save")}
          </Button>
        </>
      }
    >
      {account ? (
        <AppForm<EditValues>
          id={EDIT_FORM}
          schema={schema}
          defaultValues={{
            display_name: account.display_name,
            sandbox: account.mode === "sandbox",
            credentials: Object.fromEntries(fields.map((f) => [f.key, ""])),
          }}
          onSubmit={async (values) => {
            const credentials: Record<string, string> = {};
            for (const f of fields) {
              const v = (values.credentials[f.key] ?? "").trim();
              if (v) credentials[f.key] = v;
            }
            await update.mutateAsync({
              uuid: account.uuid,
              body: {
                display_name: values.display_name.trim(),
                mode: values.sandbox ? "sandbox" : "live",
                credentials: Object.keys(credentials).length
                  ? credentials
                  : null,
              },
            });
            onClose();
          }}
        >
          <div className="space-y-4">
            <AppInput
              name="display_name"
              label={t("integrations.fields.display_name")}
            />
            <AppSwitch
              name="sandbox"
              label={t("integrations.fields.sandbox")}
              description={t("integrations.fields.sandbox_hint")}
            />
            {fields.map((f) => (
              <CredentialInput
                key={f.key}
                field={f}
                placeholder={account.credentials_hint[f.key] ?? ""}
                description={t("integrations.rotate_hint")}
              />
            ))}
            <WebhookAddress account={account} />
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
