"use client";

import { Download, FileText, Paperclip, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { permissions } from "@/config/permissions";
import {
  DOCUMENT_ACCEPT,
  MAX_DOCUMENT_BYTES,
} from "@/features/finance/constants";
import {
  useDownloadFinanceDocument,
  useRemoveFinanceDocument,
  useUploadFinanceDocument,
} from "@/features/finance/hooks/use-finance";
import type { FinanceEntry } from "@/features/finance/types";
import { fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Receipt / invoice of a record (PDF or image, up to 10 MiB). The file is
 * stored and streamed by the API; replacing or removing it keeps the
 * stored object for the audit trail.
 */
export function EntryDocumentCard({ entry }: { entry: FinanceEntry }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite =
    can(permissions.tenant.finance.write) && entry.entry_type === "entry";
  const canDownload = can(permissions.tenant.document.download);
  const fileInput = useRef<HTMLInputElement>(null);
  const upload = useUploadFinanceDocument(entry.uuid);
  const remove = useRemoveFinanceDocument(entry.uuid);
  const downloadDoc = useDownloadFinanceDocument(entry.uuid);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const doc = entry.document;

  return (
    <EntitySectionCard
      title={t("finance.document.title")}
      actions={
        <>
          {canWrite ? (
            <Input
              ref={fileInput}
              type="file"
              accept={DOCUMENT_ACCEPT}
              className="hidden"
              aria-label={t("finance.document.attach")}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) return;
                if (file.size > MAX_DOCUMENT_BYTES) {
                  appToast.error(t("finance.document.too_large"));
                  return;
                }
                upload.mutate(file);
              }}
            />
          ) : null}
          {doc && canDownload ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={downloadDoc.isPending}
              onClick={() => downloadDoc.mutate(doc.name)}
            >
              <Download className="size-4" />
              {t("finance.document.download")}
            </Button>
          ) : null}
          {doc && canWrite ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="text-destructive hover:text-destructive"
              onClick={() => setConfirmRemove(true)}
            >
              <Trash2 className="size-4" />
              {t("finance.document.remove")}
            </Button>
          ) : null}
          {canWrite ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={upload.isPending}
              onClick={() => fileInput.current?.click()}
            >
              <Paperclip className="size-4" />
              {doc
                ? t("finance.document.replace")
                : t("finance.document.attach")}
            </Button>
          ) : null}
        </>
      }
    >
      {doc ? (
        <div className="flex items-center gap-3 rounded-md border p-3">
          <FileText className="text-muted-foreground size-5 shrink-0" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{doc.name}</p>
            <p className="text-muted-foreground text-xs">
              {doc.content_type} · {fileSize(doc.size_bytes)}
            </p>
          </div>
        </div>
      ) : (
        <EmptyState
          className="py-8"
          title={t("finance.document.empty")}
          description={t("finance.document.hint")}
        />
      )}
      <ConfirmDialog
        open={confirmRemove}
        title={t("finance.document.remove_title")}
        description={t("finance.document.remove_description")}
        confirmLabel={t("finance.document.remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setConfirmRemove(false)}
        onConfirm={() =>
          remove.mutate(undefined, {
            onSettled: () => setConfirmRemove(false),
          })
        }
      />
    </EntitySectionCard>
  );
}
