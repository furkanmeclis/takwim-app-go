"use client";

import { Upload } from "lucide-react";
import { useState } from "react";

import { ToolbarIconButton } from "@/components/tables/toolbar-icon-button";
import { ImportWizard } from "@/features/io/components/import-wizard";
import type { IoResource } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

type ImportButtonProps = {
  resource: IoResource;
  onComplete?: () => void;
  disabled?: boolean;
};

export function ImportButton({
  resource,
  onComplete,
  disabled,
}: ImportButtonProps) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);

  return (
    <>
      <ToolbarIconButton
        label={t("imports.menu_label")}
        onClick={() => setOpen(true)}
        disabled={disabled}
      >
        <Upload className="size-4" />
      </ToolbarIconButton>
      <ImportWizard
        resource={resource}
        open={open}
        onOpenChange={setOpen}
        onComplete={() => {
          onComplete?.();
        }}
      />
    </>
  );
}
