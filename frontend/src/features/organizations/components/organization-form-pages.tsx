"use client";

import { Building2, Link2, Mail, UserRound } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppRadioGroup,
  AppSelect,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useCreateOrganization,
  useOrganization,
  useUpdateOrganization,
} from "@/features/organizations/hooks/use-organizations";
import { slugFromName } from "@/features/organizations/lib/slug";
import {
  createOrganizationEditSchema,
  createOrganizationSchema,
  type OrganizationEditValues,
  type OrganizationFormValues,
} from "@/features/organizations/schemas/organization-form";
import { SECTOR_CODES, SECTOR_LABEL_KEYS } from "@/features/sectors/constants";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import { useLocale } from "@/providers/locale-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("organizations.forbidden")}
    />
  );
}

/** Keeps the slug in sync with the name until the user edits the slug. */
function SlugAutofill() {
  const { setValue, getFieldState, formState } =
    useFormContext<OrganizationFormValues>();
  const name = useWatch<OrganizationFormValues, "name">({ name: "name" });
  const touched = useRef(false);
  const slugDirty = getFieldState("slug", formState).isDirty;
  useEffect(() => {
    if (slugDirty) touched.current = true;
  }, [slugDirty]);
  useEffect(() => {
    if (touched.current) return;
    setValue("slug", slugFromName(name ?? ""), { shouldDirty: false });
  }, [name, setValue]);
  return null;
}

function SectorField() {
  const { t } = useLocale();
  return (
    <AppSelect
      name="sector"
      label={t("sectors.label")}
      description={t("sectors.field_hint")}
      options={SECTOR_CODES.map((code) => ({
        value: code,
        label: t(SECTOR_LABEL_KEYS[code]),
      }))}
    />
  );
}

function OwnerFields() {
  const { t } = useLocale();
  const mode = useWatch<OrganizationFormValues, "owner_mode">({
    name: "owner_mode",
  });
  return (
    <>
      <AppRadioGroup
        name="owner_mode"
        label={t("organizations.form.owner_mode")}
        className="sm:col-span-2"
        options={[
          {
            value: "existing",
            label: t("organizations.form.owner_existing"),
            description: t("organizations.form.owner_existing_hint"),
          },
          {
            value: "invite",
            label: t("organizations.form.owner_invite"),
            description: t("organizations.form.owner_invite_hint"),
          },
        ]}
      />
      {mode === "existing" ? (
        <UserAsyncPicker
          name="owner_user_uuid"
          label={t("organizations.form.owner_user")}
          description={t("organizations.form.owner_user_hint")}
          className="sm:col-span-2"
        />
      ) : (
        <AppInput
          name="owner_email"
          type="email"
          autoComplete="off"
          label={t("organizations.form.owner_email")}
          description={t("organizations.form.owner_email_hint")}
          placeholder="ornek@firma.com"
          startIcon={Mail}
          className="sm:col-span-2"
        />
      )}
    </>
  );
}

/** `/platform/organizations/create`: name, slug and owner (account or invitation). */
export function OrganizationCreatePage() {
  const { t } = useLocale();
  const router = useRouter();
  const create = useCreateOrganization();
  const schema = useMemo(() => createOrganizationSchema(t), [t]);
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "identity",
          label: t("organizations.form.section_identity"),
          description: t("organizations.form.section_identity_desc"),
          icon: Building2,
        },
        {
          key: "owner",
          label: t("organizations.form.section_owner"),
          description: t("organizations.form.section_owner_desc"),
          icon: UserRound,
        },
      ]),
    [t],
  );

  return (
    <EntityPage
      title={t("organizations.create_title")}
      description={t("organizations.create_description")}
      permission={permissions.organizations.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        { label: t("organizations.create_title") },
      ]}
    >
      <EntityForm>
        <AppForm<OrganizationFormValues>
          schema={schema}
          defaultValues={{
            name: "",
            slug: "",
            sector: "hairdresser",
            owner_mode: "existing",
            owner_user_uuid: "",
            owner_email: "",
          }}
          onSubmit={async (values) => {
            const result = await create.mutateAsync({
              name: values.name.trim(),
              ...(values.slug ? { slug: values.slug } : {}),
              sector: values.sector,
              ...(values.owner_mode === "existing"
                ? { owner_user_uuid: values.owner_user_uuid }
                : { owner_email: values.owner_email.trim() }),
            });
            router.push(
              routes.platform.organizations.detail(result.organization.uuid),
            );
          }}
        >
          {(form) => (
            <FormLayout navItems={sections.navItems}>
              <SlugAutofill />
              <FormSection
                id={sections.id("identity")}
                title={t("organizations.form.section_identity")}
                description={t("organizations.form.section_identity_desc")}
                columns={2}
              >
                <AppInput
                  name="name"
                  label={t("organizations.fields.name")}
                  placeholder="Örnek Güzellik Salonu"
                  startIcon={Building2}
                />
                <AppInput
                  name="slug"
                  label={t("organizations.fields.slug")}
                  description={t("organizations.form.slug_hint")}
                  startIcon={Link2}
                />
                <SectorField />
              </FormSection>
              <FormSection
                id={sections.id("owner")}
                title={t("organizations.form.section_owner")}
                description={t("organizations.form.section_owner_desc")}
                columns={2}
              >
                <OwnerFields />
              </FormSection>
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() =>
                    router.push(routes.platform.organizations.root)
                  }
                  disabled={form.formState.isSubmitting}
                >
                  {t("form.cancel")}
                </Button>
                <Button type="submit" disabled={form.formState.isSubmitting}>
                  {t("organizations.actions.create")}
                </Button>
              </FormActions>
            </FormLayout>
          )}
        </AppForm>
      </EntityForm>
    </EntityPage>
  );
}

/** `/platform/organizations/{uuid}/edit`: name and slug. */
export function OrganizationEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const query = useOrganization(uuid);
  const update = useUpdateOrganization();
  const schema = useMemo(() => createOrganizationEditSchema(t), [t]);
  const org = query.data;

  return (
    <EntityPage
      title={t("organizations.edit_title")}
      description={t("organizations.edit_description")}
      permission={permissions.organizations.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        ...(org
          ? [
              {
                label: org.name,
                href: routes.platform.organizations.detail(uuid),
              },
            ]
          : []),
        { label: t("organizations.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("organizations.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {org ? (
        <EntityForm>
          <AppForm<OrganizationEditValues>
            key={org.updated_at}
            schema={schema}
            defaultValues={{
              name: org.name,
              slug: org.slug,
              sector: org.sector,
            }}
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: {
                  name: values.name.trim(),
                  slug: values.slug,
                  sector: values.sector,
                },
              });
              router.push(routes.platform.organizations.detail(uuid));
            }}
          >
            {(form) => (
              <FormSection
                title={t("organizations.form.section_identity")}
                description={t("organizations.form.slug_change_hint")}
                columns={2}
              >
                <AppInput
                  name="name"
                  label={t("organizations.fields.name")}
                  startIcon={Building2}
                />
                <AppInput
                  name="slug"
                  label={t("organizations.fields.slug")}
                  startIcon={Link2}
                />
                <SectorField />
                <FormActions className="sm:col-span-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() =>
                      router.push(routes.platform.organizations.detail(uuid))
                    }
                    disabled={form.formState.isSubmitting}
                  >
                    {t("form.cancel")}
                  </Button>
                  <Button type="submit" disabled={form.formState.isSubmitting}>
                    {t("organizations.actions.save")}
                  </Button>
                </FormActions>
              </FormSection>
            )}
          </AppForm>
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}
