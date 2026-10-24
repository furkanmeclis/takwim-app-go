"use client";

import { PencilLine, Undo2 } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityDetail, EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import { useCashAmount } from "@/features/cash/lib/format";
import { SaleLineKindBadges } from "@/features/sales/components/sale-line-columns";
import { ITEM_TYPE_LABEL_KEYS } from "@/features/sales/constants";
import {
  useCorrectSaleLine,
  useReverseSaleLine,
  useSaleLine,
} from "@/features/sales/hooks/use-sales";
import { isLiveSale } from "@/features/sales/lib/format";
import {
  correctLineDefaults,
  correctLineToRequest,
  createCorrectLineSchema,
  type CorrectLineFormValues,
} from "@/features/sales/schemas/sales-forms";
import type { SaleLine } from "@/features/sales/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const CORRECT_FORM = "sale-line-correct-form";

/**
 * Detail drawer, reversal and correction of sale lines, shared by the
 * ledger page and the visit's balance card. `open`, `reverse` and
 * `correct` are stable setters; render `element` once.
 */
export function useSaleLineDialogs() {
  const [viewing, setViewing] = useState<SaleLine | null>(null);
  const [reversing, setReversing] = useState<SaleLine | null>(null);
  const [correcting, setCorrecting] = useState<SaleLine | null>(null);

  const element = (
    <>
      <SaleLineDrawer
        line={viewing}
        onOpenChange={(open) => {
          if (!open) setViewing(null);
        }}
        onReverse={(line) => {
          setViewing(null);
          setReversing(line);
        }}
        onCorrect={(line) => {
          setViewing(null);
          setCorrecting(line);
        }}
      />
      <ReverseLineDialog line={reversing} onClose={() => setReversing(null)} />
      <CorrectLineDialog
        line={correcting}
        onClose={() => setCorrecting(null)}
      />
    </>
  );

  return {
    open: setViewing,
    reverse: setReversing,
    correct: setCorrecting,
    element,
  };
}

function SaleLineDrawer({
  line,
  onOpenChange,
  onReverse,
  onCorrect,
}: {
  line: SaleLine | null;
  onOpenChange: (open: boolean) => void;
  onReverse: (line: SaleLine) => void;
  onCorrect: (line: SaleLine) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const money = useCashAmount();
  const query = useSaleLine(line?.uuid ?? null, line ?? undefined);
  const view = query.data;
  const canChange =
    Boolean(view && isLiveSale(view)) && can(permissions.tenant.finance.write);
  const amountOf = (value: string) => money(value, view?.currency);

  return (
    <EntityDrawer
      open={Boolean(line)}
      onOpenChange={onOpenChange}
      title={t("sales.drawer.title")}
      description={t("sales.drawer.description")}
      footer={
        canChange && view ? (
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onCorrect(view)}
            >
              <PencilLine className="size-4" />
              {t("sales.actions.correct")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={() => onReverse(view)}
            >
              <Undo2 className="size-4" />
              {t("sales.actions.reverse")}
            </Button>
          </div>
        ) : undefined
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError && !view ? (
        <ErrorState
          title={t("sales.drawer.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {view ? (
        <EntityDetail
          sections={[
            {
              id: "line",
              title: t("sales.drawer.section_line"),
              fields: [
                {
                  key: "kind",
                  label: t("sales.columns.kind"),
                  value: <SaleLineKindBadges line={view} />,
                },
                {
                  key: "description",
                  label: t("sales.columns.description"),
                  value: view.description || "—",
                },
                {
                  key: "item_type",
                  label: t("sales.columns.item_type"),
                  value: t(ITEM_TYPE_LABEL_KEYS[view.item_type]),
                },
                {
                  key: "sale_date",
                  label: t("sales.columns.sale_date"),
                  value: date(view.sale_date, undefined, locale),
                },
                {
                  key: "customer",
                  label: t("sales.columns.customer"),
                  value: view.customer ? (
                    <Link
                      className="underline-offset-4 hover:underline"
                      href={routes.tenant.customers.detail(
                        slug,
                        view.customer.uuid,
                      )}
                    >
                      {view.customer.name}
                    </Link>
                  ) : (
                    t("sales.walk_in")
                  ),
                },
                {
                  key: "reservation",
                  label: t("sales.drawer.reservation"),
                  value: view.reservation ? (
                    <Link
                      className="underline-offset-4 hover:underline"
                      href={routes.tenant.reservations.detail(
                        slug,
                        view.reservation.uuid,
                      )}
                    >
                      {t("sales.reservation_number", {
                        number: view.reservation.number,
                      })}
                    </Link>
                  ) : (
                    "—"
                  ),
                },
                {
                  key: "staff",
                  label: t("sales.columns.staff"),
                  value: view.staff?.name ?? "—",
                },
                {
                  key: "branch",
                  label: t("sales.columns.branch"),
                  value: view.branch.name,
                },
              ],
            },
            {
              id: "amounts",
              title: t("sales.drawer.section_amounts"),
              fields: [
                {
                  key: "quantity",
                  label: t("sales.columns.quantity"),
                  value: view.quantity,
                },
                {
                  key: "unit_price",
                  label: t("sales.columns.unit_price"),
                  value: amountOf(view.unit_price),
                },
                {
                  key: "gross_amount",
                  label: t("sales.drawer.gross_amount"),
                  value: amountOf(view.gross_amount),
                },
                {
                  key: "discount_amount",
                  label: t("sales.columns.discount_amount"),
                  value: amountOf(view.discount_amount),
                },
                {
                  key: "net_amount",
                  label: t("sales.columns.net_amount"),
                  value: (
                    <span className="font-semibold">
                      {amountOf(view.net_amount)}
                    </span>
                  ),
                },
              ],
            },
            {
              id: "record",
              title: t("sales.drawer.section_record"),
              fields: [
                {
                  key: "generation",
                  label: t("sales.drawer.generation"),
                  value: view.generation,
                },
                ...(view.reason
                  ? [
                      {
                        key: "reason",
                        label: t("sales.columns.reason"),
                        value: view.reason,
                      },
                    ]
                  : []),
                {
                  key: "created_by",
                  label: t("sales.drawer.created_by"),
                  value: view.created_by
                    ? `${view.created_by.name} ${view.created_by.surname}`.trim()
                    : t("sales.drawer.system"),
                },
                {
                  key: "created_at",
                  label: t("sales.columns.created_at"),
                  value: datetime(view.created_at, undefined, locale),
                },
              ],
            },
          ]}
        />
      ) : null}
    </EntityDrawer>
  );
}

function ReverseLineDialog({
  line,
  onClose,
}: {
  line: SaleLine | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const reverse = useReverseSaleLine();
  return (
    <ReasonDialog
      open={Boolean(line)}
      title={t("sales.reverse_dialog.title")}
      description={t("sales.reverse_dialog.description")}
      confirmLabel={t("sales.actions.reverse")}
      isPending={reverse.isPending}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      onConfirm={(reason) => reverse.mutateAsync({ uuid: line!.uuid, reason })}
    />
  );
}

/**
 * Quantity, unit price and discount of a live line. The price needs
 * `tenant.price.override`, the discount `tenant.discount.apply`; a field
 * the member may not change stays read-only and is never sent.
 */
function CorrectLineDialog({
  line,
  onClose,
}: {
  line: SaleLine | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const money = useCashAmount();
  const correct = useCorrectSaleLine();
  const canPrice = can(permissions.tenant.price.override);
  const canDiscount = can(permissions.tenant.discount.apply);
  const schema = useMemo(
    () => (line ? createCorrectLineSchema(t, line) : null),
    [line, t],
  );

  return (
    <AppDialog
      open={Boolean(line)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("sales.correct_dialog.title")}
      description={
        line
          ? t("sales.correct_dialog.description", {
              net: money(line.net_amount, line.currency),
            })
          : undefined
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={CORRECT_FORM}
            disabled={correct.isPending}
          >
            {t("sales.correct_dialog.submit")}
          </Button>
        </>
      }
    >
      {line && schema ? (
        <AppForm<CorrectLineFormValues>
          key={line.uuid}
          id={CORRECT_FORM}
          className="grid gap-4"
          schema={schema}
          defaultValues={correctLineDefaults(line)}
          onSubmit={async (values) => {
            await correct.mutateAsync({
              uuid: line.uuid,
              body: correctLineToRequest(line, values).body,
            });
            onClose();
          }}
        >
          <AppInput
            name="quantity"
            inputMode="numeric"
            label={t("sales.columns.quantity")}
          />
          <AppInput
            name="unit_price"
            inputMode="decimal"
            readOnly={!canPrice}
            label={t("sales.correct_dialog.unit_price", {
              currency: line.currency,
            })}
            description={
              canPrice ? undefined : t("sales.correct_dialog.price_locked")
            }
          />
          <AppInput
            name="discount_amount"
            inputMode="decimal"
            readOnly={!canDiscount}
            label={t("sales.correct_dialog.discount_amount", {
              currency: line.currency,
            })}
            description={
              canDiscount
                ? undefined
                : t("sales.correct_dialog.discount_locked")
            }
          />
          <AppTextarea
            name="reason"
            label={t("sales.fields.reason")}
            description={t("sales.correct_dialog.reason_hint")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
