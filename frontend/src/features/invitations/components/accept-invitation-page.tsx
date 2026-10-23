"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Building2, LogIn, LogOut, UserPlus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { signIn, useSession } from "next-auth/react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { Loading } from "@/components/common/loading";
import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { routes } from "@/config/routes";
import { AuthCard } from "@/features/auth/components/auth-card";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { maskEmail } from "@/features/invitations/lib/mask-email";
import { invitationAcceptService } from "@/features/invitations/services/invitations.service";
import type {
  AcceptInvitationResult,
  InvitationPreview,
} from "@/features/invitations/types";
import { isApiError } from "@/lib/api";
import {
  resolveCredentialErrorCode,
  type CredentialSignInResult,
} from "@/lib/auth/credentials-errors";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { date } from "@/lib/utils/format";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

type Translate = (key: string) => string;

function createRegisterSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("invitations.accept.validation.name")),
    surname: z
      .string()
      .trim()
      .min(1, t("invitations.accept.validation.surname")),
    password: z
      .string()
      .min(8, t("invitations.accept.validation.password"))
      .max(128, t("invitations.accept.validation.password")),
  });
}

type RegisterValues = z.infer<ReturnType<typeof createRegisterSchema>>;

const CLOSED_KEYS: Record<string, { title: string; body: string }> = {
  expired: {
    title: "invitations.accept.expired_title",
    body: "invitations.accept.expired_body",
  },
  accepted: {
    title: "invitations.accept.used_title",
    body: "invitations.accept.used_body",
  },
  revoked: {
    title: "invitations.accept.revoked_title",
    body: "invitations.accept.revoked_body",
  },
};

const ERROR_KEYS: Record<string, string> = {
  INVITATION_EXPIRED: "invitations.accept.expired_body",
  INVITATION_USED: "invitations.accept.used_body",
  INVITATION_REVOKED: "invitations.accept.revoked_body",
  INVITATION_EMAIL_MISMATCH: "invitations.accept.mismatch_body",
  ACCOUNT_EXISTS: "invitations.accept.account_exists",
  RATE_LIMITED: "invitations.accept.rate_limited",
};

function errorMessage(t: Translate, error: unknown) {
  if (isApiError(error) && ERROR_KEYS[error.code]) {
    return t(ERROR_KEYS[error.code]);
  }
  return t("invitations.accept.failed");
}

function OrganizationBadge({ preview }: { preview: InvitationPreview }) {
  const src = resolveLogoSrc(preview.organization.logo_url);
  const initials = preview.organization.name.slice(0, 2).toLocaleUpperCase();
  return (
    <div className="flex flex-col items-center gap-2 text-center">
      <Avatar className="size-14 rounded-xl border">
        {src ? (
          <AvatarImage src={src} alt="" className="object-contain" />
        ) : null}
        <AvatarFallback className="rounded-xl">
          {initials || <Building2 className="size-6" />}
        </AvatarFallback>
      </Avatar>
      <p className="text-lg font-semibold">{preview.organization.name}</p>
    </div>
  );
}

/**
 * `/invite/{token}`: public preview of a staff invitation. Signed in with
 * the invited e-mail → accept and go to `/t/{slug}`; signed in with another
 * account → explain and offer sign out; signed out → sign in (returns here)
 * or create the account for the invited e-mail.
 */
export function AcceptInvitationPage({ token }: { token: string }) {
  const { t, locale } = useLocale();
  const router = useRouter();
  const {
    user,
    isAuthenticated,
    bootstrapped,
    logout,
    markAuthenticated,
    hydrateProfile,
  } = useAuth();
  const { status: sessionStatus, update: updateSession } = useSession();
  const [mode, setMode] = useState<"choose" | "register">("choose");
  const [formError, setFormError] = useState<string | null>(null);
  const registerSchema = useMemo(() => createRegisterSchema(t), [t]);
  const returnHere = `${routes.guest.login}?next=${encodeURIComponent(`/invite/${token}`)}`;

  const preview = useQuery({
    queryKey: ["invitations", "preview", token],
    queryFn: () => invitationAcceptService.preview(token),
    retry: false,
  });

  const goToOrganization = async (result: AcceptInvitationResult) => {
    await hydrateProfile();
    router.replace(routes.tenant.home(result.organization_slug));
  };

  const accept = useMutation({
    mutationFn: () => invitationAcceptService.accept(token),
    onSuccess: (result) => void goToOrganization(result),
    onError: (error) => setFormError(errorMessage(t, error)),
  });

  const register = useMutation({
    mutationFn: (values: RegisterValues) =>
      invitationAcceptService.register(token, values),
  });

  const sessionLoading =
    sessionStatus === "loading" || (!bootstrapped && isAuthenticated);

  let body: React.ReactNode;
  if (preview.isLoading || sessionLoading) {
    body = <Loading label={t("common.loading")} />;
  } else if (preview.isError || !preview.data) {
    body = (
      <AuthCard
        title={t("invitations.accept.not_found_title")}
        description={t("invitations.accept.not_found_body")}
      >
        <Button asChild variant="outline" className="w-full">
          <Link href={routes.account.home}>
            {t("invitations.accept.go_home")}
          </Link>
        </Button>
      </AuthCard>
    );
  } else {
    const data = preview.data;
    const closed = CLOSED_KEYS[data.status];
    const inviter = data.inviter_name || t("invitations.accept.someone");
    const description = t("invitations.accept.description", {
      inviter,
      organization: data.organization.name,
    });
    const mismatch =
      isAuthenticated && user
        ? maskEmail(user.email) !== data.email_masked
        : false;

    let content: React.ReactNode;
    if (closed) {
      content = (
        <div className="space-y-4">
          <Alert>
            <AlertTitle>{t(closed.title)}</AlertTitle>
            <AlertDescription>{t(closed.body)}</AlertDescription>
          </Alert>
          <Button asChild variant="outline" className="w-full">
            <Link href={routes.account.home}>
              {t("invitations.accept.go_home")}
            </Link>
          </Button>
        </div>
      );
    } else if (isAuthenticated && user && mismatch) {
      content = (
        <div className="space-y-4">
          <Alert variant="destructive">
            <AlertTitle>{t("invitations.accept.mismatch_title")}</AlertTitle>
            <AlertDescription>
              {t("invitations.accept.mismatch_detail", {
                current: user.email,
                invited: data.email_masked,
              })}
            </AlertDescription>
          </Alert>
          <Button
            type="button"
            className="w-full"
            onClick={async () => {
              await logout();
              setFormError(null);
            }}
          >
            <LogOut className="size-4" />
            {t("invitations.accept.sign_out")}
          </Button>
        </div>
      );
    } else if (isAuthenticated && user) {
      content = (
        <div className="space-y-4">
          <p className="text-muted-foreground text-center text-sm">
            {t("invitations.accept.signed_in_as", { email: user.email })}
          </p>
          {formError ? (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          <Button
            type="button"
            className="w-full"
            disabled={accept.isPending}
            onClick={() => {
              setFormError(null);
              accept.mutate();
            }}
          >
            {t("invitations.accept.accept")}
          </Button>
        </div>
      );
    } else if (mode === "register") {
      content = (
        <AppForm<RegisterValues>
          className="space-y-4"
          schema={registerSchema}
          defaultValues={{ name: "", surname: "", password: "" }}
          onSubmit={async (values) => {
            setFormError(null);
            let result: AcceptInvitationResult;
            try {
              result = await register.mutateAsync(values);
            } catch (error) {
              setFormError(errorMessage(t, error));
              if (isApiError(error) && error.code === "ACCOUNT_EXISTS") {
                setMode("choose");
              }
              return;
            }
            // Sign in with the new password; the account is already a member.
            const signedIn = (await signIn("credentials", {
              email: result.email,
              password: values.password,
              totp_code: "",
              redirect: false,
            })) as CredentialSignInResult | undefined;
            if (!signedIn || resolveCredentialErrorCode(signedIn)) {
              router.replace(
                `${routes.guest.login}?next=${encodeURIComponent(routes.tenant.home(result.organization_slug))}`,
              );
              return;
            }
            await updateSession();
            markAuthenticated();
            await goToOrganization(result);
          }}
        >
          <div className="bg-muted/50 rounded-md border px-3 py-2 text-sm">
            <span className="text-muted-foreground">
              {t("invitations.accept.email_label")}:{" "}
            </span>
            <span className="font-medium">{data.email_masked}</span>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <AppInput
              name="name"
              autoComplete="given-name"
              label={t("invitations.accept.fields.name")}
            />
            <AppInput
              name="surname"
              autoComplete="family-name"
              label={t("invitations.accept.fields.surname")}
            />
          </div>
          <AppPassword
            name="password"
            autoComplete="new-password"
            label={t("invitations.accept.fields.password")}
            description={t("invitations.accept.fields.password_hint")}
          />
          {formError ? (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          <Button
            type="submit"
            className="w-full"
            disabled={register.isPending}
          >
            <UserPlus className="size-4" />
            {t("invitations.accept.create_account")}
          </Button>
          <Button
            type="button"
            variant="ghost"
            className="w-full"
            onClick={() => {
              setFormError(null);
              setMode("choose");
            }}
          >
            {t("form.cancel")}
          </Button>
          <p className="text-muted-foreground text-center text-xs">
            {t("invitations.accept.verification_hint")}
          </p>
        </AppForm>
      );
    } else {
      content = (
        <div className="space-y-4">
          {formError ? (
            <Alert>
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          <Button asChild className="w-full">
            <Link href={returnHere}>
              <LogIn className="size-4" />
              {t("invitations.accept.sign_in")}
            </Link>
          </Button>
          <div className="flex items-center gap-3">
            <Separator className="flex-1" />
            <span className="text-muted-foreground text-xs">
              {t("invitations.accept.or")}
            </span>
            <Separator className="flex-1" />
          </div>
          <Button
            type="button"
            variant="outline"
            className="w-full"
            onClick={() => {
              setFormError(null);
              setMode("register");
            }}
          >
            <UserPlus className="size-4" />
            {t("invitations.accept.register")}
          </Button>
          <p className="text-muted-foreground text-center text-xs">
            {t("invitations.accept.sign_in_hint", { email: data.email_masked })}
          </p>
        </div>
      );
    }

    body = (
      <AuthCard
        title={t("invitations.accept.title")}
        description={description}
        footer={
          !closed
            ? t("invitations.accept.expires", {
                date: date(data.expires_at, undefined, locale),
              })
            : undefined
        }
      >
        <div className="space-y-6">
          <OrganizationBadge preview={data} />
          {content}
        </div>
      </AuthCard>
    );
  }

  return <AuthShell>{body}</AuthShell>;
}
