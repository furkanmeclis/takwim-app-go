"use client";

import { useMemo } from "react";
import { useWatch } from "react-hook-form";

import { EntityDrawer } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppRadioGroup,
  type MultiComboboxOption,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useCreateInvitation } from "@/features/invitations/hooks/use-invitations";
import {
  createInviteSchema,
  inviteDefaults,
  inviteRequest,
  type InviteFormValues,
} from "@/features/invitations/schemas/invite-form";
import type { InvitationScope } from "@/features/invitations/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "invite-member";

function BranchPicker({ options }: { options: MultiComboboxOption[] }) {
  const { t } = useLocale();
  const scope = useWatch<InviteFormValues, "branch_scope">({
    name: "branch_scope",
  });
  if (scope !== "selected") return null;
  return (
    <AppMultiCombobox
      name="branch_uuids"
      label={t("invitations.fields.branches")}
      description={t("invitations.fields.branches_hint")}
      options={options}
      placeholder={t("invitations.fields.branches_placeholder")}
      emptyText={t("invitations.fields.branches_empty")}
    />
  );
}

/**
 * Invite a staff member by e-mail: one or more roles and the branch access
 * the account gets on acceptance. The API applies the same "only grant what
 * you hold" rules as member updates; a branch-restricted caller cannot offer
 * "every branch" (`restricted`).
 */
export function InviteDrawer({
  open,
  onOpenChange,
  scope,
  roleOptions,
  branchOptions,
  restricted = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: InvitationScope;
  roleOptions: MultiComboboxOption[];
  branchOptions: MultiComboboxOption[];
  restricted?: boolean;
}) {
  const { t } = useLocale();
  const create = useCreateInvitation(scope);
  const schema = useMemo(() => createInviteSchema(t), [t]);

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={t("invitations.create.title")}
      description={t("invitations.create.description")}
      size="md"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("invitations.actions.send")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<InviteFormValues>
          id={FORM_ID}
          className="space-y-5 px-4"
          schema={schema}
          defaultValues={inviteDefaults(restricted ? "selected" : "all")}
          onSubmit={async (values) => {
            await create.mutateAsync(inviteRequest(values));
            onOpenChange(false);
          }}
        >
          <AppInput
            name="email"
            type="email"
            autoComplete="off"
            label={t("invitations.fields.email")}
            placeholder={t("invitations.fields.email_placeholder")}
          />
          <AppMultiCombobox
            name="role_uuids"
            label={t("invitations.fields.roles")}
            description={t("invitations.fields.roles_hint")}
            options={roleOptions}
            placeholder={t("invitations.fields.roles_placeholder")}
            emptyText={t("invitations.fields.roles_empty")}
          />
          <AppRadioGroup
            name="branch_scope"
            label={t("invitations.fields.branch_scope")}
            options={[
              {
                value: "all",
                label: t("invitations.branch_scope.all"),
                disabled: restricted,
              },
              {
                value: "selected",
                label: t("invitations.branch_scope.selected"),
              },
            ]}
          />
          {restricted ? (
            <Alert>
              <AlertDescription>
                {t("invitations.create.restricted_hint")}
              </AlertDescription>
            </Alert>
          ) : null}
          <BranchPicker options={branchOptions} />
          <p className="text-muted-foreground text-sm">
            {t("invitations.create.expiry_hint")}
          </p>
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
