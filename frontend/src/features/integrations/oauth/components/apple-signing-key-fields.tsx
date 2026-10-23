"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useFormContext } from "react-hook-form";

import { AppInput } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { OAuthProviderFormValues } from "@/features/integrations/oauth/schemas/oauth-provider-form";
import type { OAuthProviderSettings } from "@/features/integrations/oauth/services/oauth-provider.service";
import { useLocale } from "@/providers/locale-provider";

// Apple names downloaded keys AuthKey_<KEYID>.p8.
const KEY_FILE_NAME = /AuthKey_([A-Za-z0-9]{10})\.p8$/i;
const MAX_KEY_FILE_BYTES = 16 * 1024;

type AppleSigningKeyFieldsProps = {
  settings: OAuthProviderSettings;
  disabled: boolean;
};

export function AppleSigningKeyFields({
  settings,
  disabled,
}: AppleSigningKeyFieldsProps) {
  const { t } = useLocale();
  // Destructure: the context object itself is not referentially stable.
  const { setValue, getValues, watch } =
    useFormContext<OAuthProviderFormValues>();
  const fileInputId = useId();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);

  const removing = watch("remove_private_key") === true;
  const pendingKey = Boolean(watch("private_key"));
  const configured = settings.private_key_configured === true;
  const source = settings.private_key_source ?? "none";

  // After a save the settings object is refreshed: drop the uploaded key text
  // and the remove flag so they are not sent again, and show stored ids.
  useEffect(() => {
    if (fileInputRef.current) fileInputRef.current.value = "";
    setValue("private_key", "");
    setValue("remove_private_key", false);
    setValue("team_id", settings.team_id ?? "");
    setValue("key_id", settings.key_id ?? "");
  }, [setValue, settings]);

  const clearFile = () => {
    setFileName(null);
    if (fileInputRef.current) fileInputRef.current.value = "";
    setValue("private_key", "", { shouldDirty: true });
  };

  const onFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    setFileError(null);
    const file = event.target.files?.[0];
    if (!file) {
      clearFile();
      return;
    }
    if (file.size > MAX_KEY_FILE_BYTES) {
      setFileError(t("integrations.apple.form.private_key_invalid"));
      clearFile();
      return;
    }
    const text = await file.text();
    if (!text.includes("-----BEGIN PRIVATE KEY-----")) {
      setFileError(t("integrations.apple.form.private_key_invalid"));
      clearFile();
      return;
    }
    setFileName(file.name);
    setValue("private_key", text, { shouldDirty: true });
    setValue("remove_private_key", false, { shouldDirty: true });
    const match = KEY_FILE_NAME.exec(file.name);
    if (match && !getValues("key_id")?.trim()) {
      setValue("key_id", match[1].toUpperCase(), { shouldDirty: true });
    }
  };

  let status: { label: string; variant: "default" | "secondary" | "outline" };
  if (removing) {
    status = {
      label: t("integrations.apple.form.private_key_status_removing"),
      variant: "outline",
    };
  } else if (configured) {
    status = {
      label: t("integrations.apple.form.private_key_status_configured", {
        keyId: settings.key_id ?? "",
      }),
      variant: "default",
    };
  } else if (source === "env") {
    status = {
      label: t("integrations.apple.form.private_key_status_env"),
      variant: "secondary",
    };
  } else {
    status = {
      label: t("integrations.apple.form.private_key_status_missing"),
      variant: "outline",
    };
  }

  return (
    <>
      <AppInput
        name="team_id"
        label={t("integrations.apple.form.team_id")}
        description={t("integrations.apple.form.team_id_hint")}
        maxLength={10}
        autoComplete="off"
        disabled={disabled}
      />

      <AppInput
        name="key_id"
        label={t("integrations.apple.form.key_id")}
        description={t("integrations.apple.form.key_id_hint")}
        maxLength={10}
        autoComplete="off"
        disabled={disabled}
      />

      <div className="space-y-2 sm:col-span-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Label htmlFor={fileInputId}>
            {t("integrations.apple.form.private_key")}
          </Label>
          <Badge variant={status.variant}>{status.label}</Badge>
        </div>
        <Input
          ref={fileInputRef}
          id={fileInputId}
          type="file"
          accept=".p8"
          onChange={onFileChange}
          disabled={disabled}
        />
        <p className="text-muted-foreground text-sm">
          {fileName && pendingKey
            ? t("integrations.apple.form.private_key_selected", {
                name: fileName,
              })
            : configured
              ? t("integrations.apple.form.private_key_replace_hint")
              : t("integrations.apple.form.private_key_hint")}
        </p>
        {fileError ? (
          <p className="text-destructive text-sm">{fileError}</p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {pendingKey ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={clearFile}
              disabled={disabled}
            >
              {t("integrations.apple.form.private_key_clear_selection")}
            </Button>
          ) : null}
          {configured && !pendingKey ? (
            <Button
              type="button"
              variant={removing ? "outline" : "destructive"}
              size="sm"
              onClick={() =>
                setValue("remove_private_key", !removing, {
                  shouldDirty: true,
                })
              }
              disabled={disabled}
            >
              {removing
                ? t("integrations.apple.form.private_key_keep")
                : t("integrations.apple.form.private_key_remove")}
            </Button>
          ) : null}
        </div>
      </div>
    </>
  );
}
