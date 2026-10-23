"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { signIn, signOut, useSession } from "next-auth/react";
import { signIn as signInPasskey } from "next-auth/webauthn";
import { useQuery } from "@tanstack/react-query";
import { Fingerprint, Mail, QrCode, ShieldCheck } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { brand } from "@/config/brand";
import { AppleIcon } from "@/components/icons/apple-icon";
import { FacebookIcon } from "@/components/icons/facebook-icon";
import { GitHubIcon } from "@/components/icons/github-icon";
import { GoogleIcon } from "@/components/icons/google-icon";
import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldGroup } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import { routes } from "@/config/routes";
import {
  createLoginSchema,
  type LoginFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import {
  CREDENTIAL_ERROR_CODES,
  resolveCredentialErrorCode,
  type CredentialSignInResult,
} from "@/lib/auth/credentials-errors";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { fetchAppPublicConfig } from "@/services/app-config.service";
import { authService } from "@/services/auth.service";
import { z } from "zod";

import { AuthCard } from "./auth-card";
import { QRLoginPanel } from "./qr-login-panel";

function resolveNext(raw: string | null, fallback: string) {
  // Only same-origin paths. Browsers treat a backslash like "/", so a value
  // starting "/<backslash>evil.com" would become a protocol-relative redirect.
  if (
    !raw ||
    !raw.startsWith("/") ||
    raw.startsWith("//") ||
    /[\\\u0000-\u001f]/.test(raw)
  ) {
    return fallback;
  }
  return raw;
}

type OAuthId = "github" | "google" | "facebook" | "apple";

type SavedCredentials =
  | { kind: "password"; email: string; password: string }
  | { kind: "email-code"; email: string; code: string };

type EmailCodeStep = "off" | "email" | "code";

function createEmailCodeEmailSchema(t: (key: string) => string) {
  return z.object({
    email: z.string().trim().email(t("auth.validation.email")),
  });
}

function createEmailCodeSchema(t: (key: string) => string) {
  return z.object({
    code: z
      .string()
      .trim()
      .regex(/^\d{6}$/, t("auth.email_code.validation.code")),
  });
}

function createMfaSchema(t: (key: string) => string) {
  return z.object({
    totp_code: z.string().min(1, t("auth.totp.validation.required")),
  });
}

export function LoginForm() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { markAuthenticated, hydrateProfile } = useAuth();
  const { status: sessionStatus, update: updateSession } = useSession();
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [passkeyPending, setPasskeyPending] = useState(false);
  const [oauthPending, setOauthPending] = useState<OAuthId | null>(null);
  const [mfaStep, setMfaStep] = useState(false);
  const [savedCredentials, setSavedCredentials] =
    useState<SavedCredentials | null>(null);
  const schema = useMemo(() => createLoginSchema(t), [t]);
  const mfaSchema = useMemo(() => createMfaSchema(t), [t]);
  const emailCodeEmailSchema = useMemo(
    () => createEmailCodeEmailSchema(t),
    [t],
  );
  const emailCodeSchema = useMemo(() => createEmailCodeSchema(t), [t]);
  const [emailCodeStep, setEmailCodeStep] = useState<EmailCodeStep>("off");
  const [emailCodeEmail, setEmailCodeEmail] = useState("");
  const [qrStep, setQrStep] = useState(false);

  const { data: appConfig } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
  });

  const methods = appConfig?.auth_methods;
  const showPasskey = methods?.passkey.login !== false;
  const showPassword = methods?.password.login !== false;
  const showRegisterLink =
    Boolean(appConfig?.registration_enabled) &&
    (methods?.password.register ||
      methods?.github.register ||
      methods?.google.register ||
      methods?.facebook.register ||
      methods?.apple.register);

  const oauthError = searchParams.get("error");
  const oauthFormError =
    oauthError === "GitHubNotLinked" ||
    oauthError === "OAuthNotLinked" ||
    oauthError === "OAuthAccountNotLinked" ||
    oauthError === "AccountNotLinked"
      ? t("auth.oauth.not_linked")
      : oauthError === "Configuration" ||
          oauthError === "AccessDenied" ||
          oauthError === "AdapterError" ||
          oauthError === "CallbackRouteError"
        ? t("auth.oauth.sign_in_error")
        : null;
  const displayError = formError ?? oauthFormError;

  const finishLogin = async () => {
    await updateSession();
    markAuthenticated();
    const user = await hydrateProfile();
    if (!user) {
      setFormError(t("auth.login.error"));
      return false;
    }
    const home = defaultHomeForUser(user);
    if (home === routes.errors.forbidden) {
      setFormError(t("errors.role_mismatch"));
      return false;
    }
    const next = resolveNext(searchParams.get("next"), home);
    router.replace(next);
    return true;
  };

  const resolveCredentialsError = (code: string | null) => {
    if (!code) return t("auth.login.error");
    if (code.includes("RATE_LIMITED")) {
      return t("auth.login.rate_limited");
    }
    if (code.includes("NO_TENANT_MEMBERSHIP")) {
      return t("auth.login.no_tenant_membership");
    }
    if (code === CREDENTIAL_ERROR_CODES.MFA_REQUIRED) {
      return t("auth.totp.login_required");
    }
    if (code === CREDENTIAL_ERROR_CODES.INVALID_MFA_CODE) {
      return t("auth.totp.login_invalid");
    }
    if (code === CREDENTIAL_ERROR_CODES.MFA_NOT_ENROLLED) {
      return t("auth.totp.login_not_enrolled");
    }
    if (code === CREDENTIAL_ERROR_CODES.ACCOUNT_DEACTIVATED) {
      return t("auth.login.account_deactivated");
    }
    if (code === CREDENTIAL_ERROR_CODES.INVALID_EMAIL_CODE) {
      return t("auth.email_code.invalid");
    }
    return t("auth.login.error");
  };

  const signInWithCredentials = async (
    credentials: SavedCredentials,
    totpCode?: string,
  ) => {
    const provider =
      credentials.kind === "password" ? "credentials" : "email-code";
    const secret =
      credentials.kind === "password"
        ? { password: credentials.password }
        : { code: credentials.code };
    const result = (await signIn(provider, {
      email: credentials.email,
      ...secret,
      totp_code: totpCode ?? "",
      redirect: false,
    })) as CredentialSignInResult | undefined;
    const errorCode = result ? resolveCredentialErrorCode(result) : null;
    if (errorCode) {
      if (errorCode === CREDENTIAL_ERROR_CODES.MFA_REQUIRED) {
        setSavedCredentials(credentials);
        setMfaStep(true);
        setFormError(null);
        return "mfa";
      }
      setFormError(resolveCredentialsError(errorCode));
      return false;
    }
    if (!result?.ok) {
      setFormError(t("auth.login.error"));
      return false;
    }
    const ok = await finishLogin();
    return ok;
  };

  const onSubmit = async (values: LoginFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      const outcome = await signInWithCredentials({
        kind: "password",
        email: values.email,
        password: values.password,
      });
      if (outcome !== true && outcome !== "mfa") setPending(false);
      else if (outcome === true) setPending(false);
    } catch {
      setFormError(t("auth.login.error"));
      setPending(false);
    }
  };

  const onMfaSubmit = async (values: z.infer<typeof mfaSchema>) => {
    if (!savedCredentials) return;
    setFormError(null);
    setPending(true);
    try {
      const outcome = await signInWithCredentials(
        savedCredentials,
        values.totp_code.trim(),
      );
      if (outcome === true) {
        setMfaStep(false);
        setSavedCredentials(null);
      }
      setPending(false);
    } catch {
      setFormError(t("auth.totp.login_invalid"));
      setPending(false);
    }
  };

  const cancelMfa = () => {
    setMfaStep(false);
    setSavedCredentials(null);
    setFormError(null);
  };

  const startEmailCode = () => {
    setFormError(null);
    setEmailCodeStep("email");
  };

  const cancelEmailCode = () => {
    setFormError(null);
    setEmailCodeStep("off");
  };

  const onEmailCodeRequest = async (
    values: z.infer<typeof emailCodeEmailSchema>,
  ) => {
    setFormError(null);
    setPending(true);
    try {
      if (sessionStatus === "authenticated") {
        await signOut({ redirect: false });
      }
      await authService.requestEmailCode(values.email.trim());
      setEmailCodeEmail(values.email.trim());
      setEmailCodeStep("code");
    } catch (error) {
      setFormError(
        isApiError(error) && error.code === "RATE_LIMITED"
          ? t("auth.login.rate_limited")
          : t("auth.email_code.request_error"),
      );
    } finally {
      setPending(false);
    }
  };

  const onEmailCodeSubmit = async (values: z.infer<typeof emailCodeSchema>) => {
    setFormError(null);
    setPending(true);
    try {
      const outcome = await signInWithCredentials({
        kind: "email-code",
        email: emailCodeEmail,
        code: values.code.trim(),
      });
      if (outcome === "mfa") setEmailCodeStep("off");
      setPending(false);
    } catch {
      setFormError(t("auth.email_code.invalid"));
      setPending(false);
    }
  };

  const onPasskeySignIn = async () => {
    setFormError(null);
    setPasskeyPending(true);
    try {
      if (sessionStatus === "authenticated") {
        await signOut({ redirect: false });
      }
      const result = await signInPasskey("passkey", {
        action: "authenticate",
        redirect: false,
      });
      if (!result || result.error) {
        if (result?.error?.includes("NO_TENANT_MEMBERSHIP")) {
          setFormError(t("auth.login.no_tenant_membership"));
        } else {
          setFormError(t("auth.passkey.error"));
        }
        setPasskeyPending(false);
        return;
      }
      const ok = await finishLogin();
      if (!ok) setPasskeyPending(false);
    } catch {
      setFormError(t("auth.passkey.error"));
      setPasskeyPending(false);
    }
  };

  const onOAuthSignIn = async (provider: OAuthId) => {
    setFormError(null);
    setOauthPending(provider);
    try {
      if (sessionStatus === "authenticated") {
        await signOut({ redirect: false });
      }
      await signIn(provider, {
        redirect: true,
        callbackUrl: resolveNext(
          searchParams.get("next"),
          routes.platform.home,
        ),
      });
    } catch {
      setFormError(t("auth.oauth.sign_in_error"));
      setOauthPending(null);
    }
  };

  const authPending = pending || passkeyPending || oauthPending !== null;

  const oauthButtons: {
    id: OAuthId;
    enabled: boolean;
    icon: ReactNode;
    label: string;
    pendingLabel: string;
  }[] = [
    {
      id: "github",
      enabled: Boolean(methods?.github.login),
      icon: <GitHubIcon className="size-4" />,
      label: t("auth.github.sign_in"),
      pendingLabel: t("auth.github.signing_in"),
    },
    {
      id: "google",
      enabled: Boolean(methods?.google.login),
      icon: <GoogleIcon className="size-4" />,
      label: t("auth.google.sign_in"),
      pendingLabel: t("auth.google.signing_in"),
    },
    {
      id: "facebook",
      enabled: Boolean(methods?.facebook.login),
      icon: <FacebookIcon className="size-4" />,
      label: t("auth.facebook.sign_in"),
      pendingLabel: t("auth.facebook.signing_in"),
    },
    {
      id: "apple",
      enabled: Boolean(methods?.apple.login),
      icon: <AppleIcon className="size-4" />,
      label: t("auth.apple.sign_in"),
      pendingLabel: t("auth.apple.signing_in"),
    },
  ];

  if (mfaStep) {
    return (
      <AuthCard
        title={t("auth.totp.login_title")}
        description={t("auth.totp.login_description")}
      >
        <AppForm
          schema={mfaSchema}
          defaultValues={{ totp_code: "" }}
          onSubmit={onMfaSubmit}
        >
          <FieldGroup>
            {displayError ? (
              <Field data-invalid={true}>
                <FieldError>{displayError}</FieldError>
              </Field>
            ) : null}

            <AppInput
              name="totp_code"
              label={t("auth.totp.confirm_code")}
              autoComplete="one-time-code"
              inputMode="numeric"
              placeholder={t("auth.totp.code_placeholder")}
            />

            <Field className="flex flex-col gap-2 sm:flex-row">
              <Button
                type="submit"
                disabled={authPending}
                className="sm:flex-1"
              >
                <ShieldCheck aria-hidden />
                {pending
                  ? t("auth.login.submitting")
                  : t("auth.totp.verify_login")}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={authPending}
                onClick={cancelMfa}
              >
                {t("common.back")}
              </Button>
            </Field>
          </FieldGroup>
        </AppForm>
      </AuthCard>
    );
  }

  if (qrStep) {
    return (
      <AuthCard
        title={t("auth.qr.title")}
        description={t("auth.qr.description", { app: brand.name })}
      >
        <div className="space-y-4">
          {displayError ? (
            <Field data-invalid={true}>
              <FieldError>{displayError}</FieldError>
            </Field>
          ) : null}
          <QRLoginPanel
            onSignedIn={finishLogin}
            onErrorCode={(code) => setFormError(resolveCredentialsError(code))}
          />
          <Button
            type="button"
            variant="outline"
            className="w-full"
            onClick={() => {
              setFormError(null);
              setQrStep(false);
            }}
          >
            {t("common.back")}
          </Button>
        </div>
      </AuthCard>
    );
  }

  if (emailCodeStep !== "off") {
    const errorBlock = displayError ? (
      <Field data-invalid={true}>
        <FieldError>{displayError}</FieldError>
      </Field>
    ) : null;
    return (
      <AuthCard
        title={t("auth.email_code.title")}
        description={
          emailCodeStep === "email"
            ? t("auth.email_code.description")
            : t("auth.email_code.sent", { email: emailCodeEmail })
        }
      >
        {emailCodeStep === "email" ? (
          <AppForm
            key="email-code-email"
            schema={emailCodeEmailSchema}
            defaultValues={{ email: emailCodeEmail }}
            onSubmit={onEmailCodeRequest}
          >
            <FieldGroup>
              {errorBlock}
              <AppInput
                name="email"
                label={t("auth.login.email")}
                type="email"
                autoComplete="email"
                placeholder={t("auth.placeholders.email")}
              />
              <Field className="flex flex-col gap-2 sm:flex-row">
                <Button
                  type="submit"
                  disabled={authPending}
                  className="sm:flex-1"
                >
                  <Mail aria-hidden />
                  {pending
                    ? t("auth.email_code.sending")
                    : t("auth.email_code.send")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={authPending}
                  onClick={cancelEmailCode}
                >
                  {t("common.back")}
                </Button>
              </Field>
            </FieldGroup>
          </AppForm>
        ) : (
          <AppForm
            key="email-code-code"
            schema={emailCodeSchema}
            defaultValues={{ code: "" }}
            onSubmit={onEmailCodeSubmit}
          >
            <FieldGroup>
              {errorBlock}
              <AppInput
                name="code"
                label={t("auth.email_code.code")}
                autoComplete="one-time-code"
                inputMode="numeric"
                maxLength={6}
                placeholder={t("auth.email_code.code_placeholder")}
              />
              <Field className="flex flex-col gap-2 sm:flex-row">
                <Button
                  type="submit"
                  disabled={authPending}
                  className="sm:flex-1"
                >
                  {pending
                    ? t("auth.login.submitting")
                    : t("auth.email_code.verify")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={authPending}
                  onClick={() => {
                    setFormError(null);
                    setEmailCodeStep("email");
                  }}
                >
                  {t("auth.email_code.change_email")}
                </Button>
              </Field>
            </FieldGroup>
          </AppForm>
        )}
      </AuthCard>
    );
  }

  return (
    <AuthCard
      title={t("auth.login.title")}
      description={t("auth.login.description")}
    >
      <div className="space-y-4">
        <Button
          type="button"
          variant="outline"
          className="w-full"
          disabled={authPending}
          onClick={() => {
            setFormError(null);
            setQrStep(true);
          }}
        >
          <QrCode aria-hidden />
          {t("auth.qr.sign_in")}
        </Button>

        {showPasskey ? (
          <Button
            type="button"
            variant="outline"
            className="w-full"
            disabled={authPending}
            onClick={onPasskeySignIn}
          >
            <Fingerprint aria-hidden />
            {passkeyPending
              ? t("auth.passkey.signing_in")
              : t("auth.passkey.sign_in")}
          </Button>
        ) : null}

        {oauthButtons
          .filter((button) => button.enabled)
          .map((button) => (
            <Button
              key={button.id}
              type="button"
              variant="outline"
              className="w-full"
              disabled={authPending}
              onClick={() => onOAuthSignIn(button.id)}
            >
              {button.icon}
              {oauthPending === button.id ? button.pendingLabel : button.label}
            </Button>
          ))}

        {showPassword ? (
          <>
            {(showPasskey || oauthButtons.some((button) => button.enabled)) && (
              <div className="flex items-center gap-3">
                <Separator className="flex-1" />
                <span className="text-muted-foreground text-xs uppercase">
                  {t("auth.passkey.or_password")}
                </span>
                <Separator className="flex-1" />
              </div>
            )}

            <AppForm
              schema={schema}
              defaultValues={{ email: "", password: "" }}
              onSubmit={onSubmit}
            >
              <FieldGroup>
                {displayError ? (
                  <Field data-invalid={true}>
                    <FieldError>{displayError}</FieldError>
                  </Field>
                ) : null}

                <AppInput
                  name="email"
                  label={t("auth.login.email")}
                  type="email"
                  autoComplete="email"
                  placeholder={t("auth.placeholders.email")}
                />
                <AppPassword
                  name="password"
                  label={t("auth.login.password")}
                  autoComplete="current-password"
                  placeholder={t("auth.placeholders.password")}
                  labelAction={
                    <Link
                      href={routes.guest.forgotPassword}
                      className="ml-auto text-sm underline-offset-4 hover:underline"
                    >
                      {t("auth.login.forgot_link")}
                    </Link>
                  }
                />

                <Field>
                  <Button type="submit" disabled={authPending}>
                    {pending
                      ? t("auth.login.submitting")
                      : t("auth.login.submit")}
                  </Button>
                </Field>
              </FieldGroup>
            </AppForm>
          </>
        ) : displayError ? (
          <Field data-invalid={true}>
            <FieldError>{displayError}</FieldError>
          </Field>
        ) : null}

        <Button
          type="button"
          variant="ghost"
          className="w-full"
          disabled={authPending}
          onClick={startEmailCode}
        >
          <Mail aria-hidden />
          {t("auth.email_code.sign_in")}
        </Button>

        {showRegisterLink ? (
          <p className="text-muted-foreground text-center text-sm">
            {t("auth.login.no_account")}{" "}
            <Link
              href={routes.guest.register}
              className="underline-offset-4 hover:underline"
            >
              {t("auth.login.register_link")}
            </Link>
          </p>
        ) : null}
      </div>
    </AuthCard>
  );
}
