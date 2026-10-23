import NextAuth from "next-auth";
import type { Provider } from "next-auth/providers";
import Apple from "next-auth/providers/apple";
import Credentials from "next-auth/providers/credentials";
import Facebook from "next-auth/providers/facebook";
import GitHub from "next-auth/providers/github";
import Google from "next-auth/providers/google";
import Passkey from "next-auth/providers/passkey";
import { NextResponse } from "next/server";

import { routes } from "@/config/routes";
import { credentialsErrorFor } from "@/lib/auth/credentials-errors";
import { goAdapter } from "@/lib/auth/go-adapter";
import {
  adapterGetOAuthConfig,
  adapterGetOAuthAccountUser,
  adapterGetUserByEmail,
  adapterIssueSession,
  exchangeQRLogin,
  loginWithEmailCode,
  loginWithPassword,
  type OAuthConfigPayload,
} from "@/lib/auth/go-adapter-client";
import { logAuthError } from "@/lib/observability/auth-logger";
import { clientIpFromHeaders } from "@/lib/server/upstream";
import { fetchAppPublicConfig } from "@/services/app-config.service";

const pendingGitHubLogins = new Map<string, string>();

const OAUTH_PROVIDERS = ["github", "google", "facebook", "apple"] as const;
type OAuthProviderId = (typeof OAUTH_PROVIDERS)[number];

const GUEST_AUTH_PREFIXES = [
  routes.guest.login,
  routes.guest.register,
  routes.guest.forgotPassword,
  routes.guest.resetPassword,
  routes.guest.verifyEmail,
];

function isGuestAuthPath(pathname: string) {
  return GUEST_AUTH_PREFIXES.some(
    (path) => pathname === path || pathname.startsWith(`${path}/`),
  );
}

function isProtectedPath(pathname: string) {
  if (pathname === routes.cms.root) return true;
  if (pathname.startsWith(`${routes.cms.profile.root}`)) return true;
  if (pathname === routes.platform.root) return true;
  if (
    pathname.startsWith(`${routes.platform.root}/`) &&
    !isGuestAuthPath(pathname)
  ) {
    return true;
  }
  return false;
}

function isOAuthProvider(
  provider: string | undefined,
): provider is OAuthProviderId {
  return (
    provider !== undefined &&
    (OAUTH_PROVIDERS as readonly string[]).includes(provider)
  );
}

// Providers are built by making several backend calls. Cache the result for
// PROVIDERS_CACHE_TTL ms so repeated session/CSRF requests don't re-issue all
// those HTTP round-trips on every hit.
const PROVIDERS_CACHE_TTL = 5 * 60 * 1000; // 5 minutes
// Generated client secrets (Apple signing key, valid 24h) must never outlive
// the cache: rebuild at least this long before the earliest expiry.
const SECRET_EXPIRY_MARGIN = 10 * 60 * 1000; // 10 minutes
let _providersCache: { providers: Provider[]; expiresAt: number } | null = null;
// Earliest client_secret_expires_at seen while building providers.
let _buildSecretExpiry: number | null = null;
let _providersBuildPromise: Promise<Provider[]> | null = null;

async function loadOAuthConfig(
  provider: OAuthProviderId,
): Promise<OAuthConfigPayload | null> {
  try {
    const config = await adapterGetOAuthConfig(provider);
    if (config.enabled && config.client_id && config.client_secret) {
      if (config.client_secret_expires_at) {
        const exp = Date.parse(config.client_secret_expires_at);
        if (Number.isFinite(exp)) {
          if (exp - SECRET_EXPIRY_MARGIN <= Date.now()) {
            return null; // already (nearly) expired: never hand it to NextAuth
          }
          _buildSecretExpiry =
            _buildSecretExpiry === null
              ? exp
              : Math.min(_buildSecretExpiry, exp);
        }
      }
      return config;
    }
  } catch {
    // Provider stays disabled when config is unavailable.
  }
  return null;
}

async function _doBuildProviders(): Promise<Provider[]> {
  let passwordLogin = true;
  let passkeyLogin = true;
  try {
    const appConfig = await fetchAppPublicConfig("en");
    passwordLogin = appConfig.auth_methods.password.login;
    passkeyLogin = appConfig.auth_methods.passkey.login;
  } catch {
    // Keep defaults when public config is unavailable.
  }

  // Fetch all OAuth configs in parallel instead of sequentially.
  const [github, google, facebook, apple] = await Promise.all([
    loadOAuthConfig("github"),
    loadOAuthConfig("google"),
    loadOAuthConfig("facebook"),
    loadOAuthConfig("apple"),
  ]);

  const providers: Provider[] = [];

  if (passwordLogin) {
    providers.push(
      Credentials({
        credentials: {
          email: { label: "Email", type: "email" },
          password: { label: "Password", type: "password" },
          totp_code: { label: "Authenticator code", type: "text" },
        },
        async authorize(credentials, request) {
          const email = String(credentials?.email ?? "").trim();
          const password = String(credentials?.password ?? "");
          const totpCode = String(credentials?.totp_code ?? "").trim();
          if (!email || !password) {
            console.error("[auth][credentials] missing email/password", {
              hasEmail: Boolean(email),
              hasPassword: Boolean(password),
            });
            return null;
          }

          try {
            const tokens = await loginWithPassword(
              email,
              password,
              totpCode || undefined,
              request instanceof Request
                ? clientIpFromHeaders(request.headers)
                : null,
            );
            const user = await adapterGetUserByEmail(email);
            return {
              id: user.id,
              email: user.email,
              name: user.name,
              accessToken: tokens.access_token,
              refreshToken: tokens.refresh_token,
              expiresIn: tokens.expires_in,
            };
          } catch (error) {
            const code = (error as Error & { code?: string }).code;
            console.error("[auth][credentials] authorize failed", {
              email,
              code,
              message: error instanceof Error ? error.message : String(error),
              status: (error as Error & { status?: number }).status,
            });
            const mapped = credentialsErrorFor(code);
            if (mapped) throw mapped;
            return null;
          }
        },
      }),
    );
  }

  // Email one-time code sign-in (always available; the Go API owns policy).
  providers.push(
    Credentials({
      id: "email-code",
      name: "Email code",
      credentials: {
        email: { label: "Email", type: "email" },
        code: { label: "Code", type: "text" },
        totp_code: { label: "Authenticator code", type: "text" },
      },
      async authorize(credentials, request) {
        const email = String(credentials?.email ?? "").trim();
        const code = String(credentials?.code ?? "").trim();
        const totpCode = String(credentials?.totp_code ?? "").trim();
        if (!email || !code) return null;
        try {
          const tokens = await loginWithEmailCode(
            email,
            code,
            totpCode || undefined,
            request instanceof Request
              ? clientIpFromHeaders(request.headers)
              : null,
          );
          const user = await adapterGetUserByEmail(email);
          return {
            id: user.id,
            email: user.email,
            name: user.name,
            accessToken: tokens.access_token,
            refreshToken: tokens.refresh_token,
            expiresIn: tokens.expires_in,
          };
        } catch (error) {
          const errCode = (error as Error & { code?: string }).code;
          const mapped = credentialsErrorFor(errCode);
          if (mapped) throw mapped;
          return null;
        }
      },
    }),
  );

  // QR sign-in: the tab redeems an approval made in the mobile app. Needs
  // the tab's browser secret plus the one-time exchange token from its
  // private realtime channel; the Go API enforces single use and expiry.
  providers.push(
    Credentials({
      id: "qr-login",
      name: "QR code",
      credentials: {
        session_id: { label: "Session", type: "text" },
        browser_secret: { label: "Browser secret", type: "text" },
        exchange_token: { label: "Exchange token", type: "text" },
      },
      async authorize(credentials, request) {
        const sessionId = String(credentials?.session_id ?? "").trim();
        const browserSecret = String(credentials?.browser_secret ?? "").trim();
        const exchangeToken = String(credentials?.exchange_token ?? "").trim();
        if (!sessionId || !browserSecret || !exchangeToken) return null;
        try {
          const result = await exchangeQRLogin(
            sessionId,
            browserSecret,
            exchangeToken,
            request instanceof Request
              ? clientIpFromHeaders(request.headers)
              : null,
          );
          return {
            id: result.user.uuid,
            email: result.user.email,
            name: result.user.name,
            accessToken: result.access_token,
            refreshToken: result.refresh_token,
            expiresIn: result.expires_in,
          };
        } catch (error) {
          const errCode = (error as Error & { code?: string }).code;
          const mapped = credentialsErrorFor(errCode);
          if (mapped) throw mapped;
          return null;
        }
      },
    }),
  );

  if (passkeyLogin) {
    providers.push(Passkey({}));
  }

  if (github) {
    providers.push(
      GitHub({
        clientId: github.client_id,
        clientSecret: github.client_secret,
      }),
    );
  }

  if (google) {
    providers.push(
      Google({
        clientId: google.client_id,
        clientSecret: google.client_secret,
      }),
    );
  }

  if (facebook) {
    providers.push(
      Facebook({
        clientId: facebook.client_id,
        clientSecret: facebook.client_secret,
      }),
    );
  }

  if (apple) {
    providers.push(
      Apple({
        clientId: apple.client_id,
        clientSecret: apple.client_secret,
      }),
    );
  }

  return providers;
}

async function buildProviders(): Promise<Provider[]> {
  const now = Date.now();
  if (_providersCache && now < _providersCache.expiresAt) {
    return _providersCache.providers;
  }
  // Deduplicate concurrent calls while the first build is in flight.
  if (!_providersBuildPromise) {
    _buildSecretExpiry = null;
    _providersBuildPromise = _doBuildProviders()
      .then((providers) => {
        let expiresAt = Date.now() + PROVIDERS_CACHE_TTL;
        if (_buildSecretExpiry !== null) {
          expiresAt = Math.min(
            expiresAt,
            _buildSecretExpiry - SECRET_EXPIRY_MARGIN,
          );
        }
        _providersCache = { providers, expiresAt };
        _providersBuildPromise = null;
        return providers;
      })
      .catch((err) => {
        _providersBuildPromise = null;
        throw err;
      });
  }
  return _providersBuildPromise;
}

// Apple answers with response_mode=form_post: a cross-site POST to
// /api/auth/callback/apple. Browsers leave SameSite=Lax cookies off such a
// request, so the state/PKCE/nonce check cookies were missing and Apple
// sign-in failed with InvalidCheck. Over https those three cookies are
// SameSite=None (which requires Secure). Fixed names keep them independent of
// the proxy's http/https detection; the session cookie is left untouched.
const secureOrigin = (process.env.AUTH_URL ?? "").startsWith("https://");
const crossSiteCheckCookie = (name: string, maxAge?: number) => ({
  name,
  options: {
    httpOnly: true,
    sameSite: "none" as const,
    path: "/",
    secure: true,
    ...(maxAge ? { maxAge } : {}),
  },
});
const oauthCheckCookies = secureOrigin
  ? {
      pkceCodeVerifier: crossSiteCheckCookie(
        "authjs.pkce.code_verifier",
        60 * 15,
      ),
      state: crossSiteCheckCookie("authjs.state", 60 * 15),
      nonce: crossSiteCheckCookie("authjs.nonce"),
    }
  : undefined;

const authHandlers = NextAuth(async () => ({
  adapter: goAdapter({ pendingGitHubLogins }),
  trustHost: process.env.AUTH_TRUST_HOST === "true",
  session: { strategy: "jwt" },
  ...(oauthCheckCookies ? { cookies: oauthCheckCookies } : {}),
  logger: { error: logAuthError },
  providers: await buildProviders(),
  experimental: {
    enableWebAuthn: true,
  },
  pages: {
    signIn: routes.guest.login,
    error: routes.guest.login,
  },
  callbacks: {
    authorized({ auth, request }) {
      const { pathname } = request.nextUrl;
      if (!isProtectedPath(pathname) || auth) {
        return true;
      }

      const url = request.nextUrl.clone();
      url.pathname = routes.guest.login;
      url.searchParams.set("next", pathname);
      return NextResponse.redirect(url);
    },
    async signIn({ account, profile }) {
      if (!isOAuthProvider(account?.provider)) {
        return true;
      }

      const provider = account.provider;

      if (
        provider === "github" &&
        profile &&
        typeof profile === "object" &&
        "login" in profile &&
        typeof profile.login === "string" &&
        account.providerAccountId
      ) {
        pendingGitHubLogins.set(account.providerAccountId, profile.login);
      }

      const providerAccountId = account.providerAccountId ?? "";
      try {
        await adapterGetOAuthAccountUser(provider, providerAccountId);
        return true;
      } catch {
        // Not linked yet.
      }

      const { getApiTokens } = await import("@/lib/server/auth-tokens");
      const session = await getApiTokens();
      if (session.userId) {
        return true;
      }

      try {
        const config = await adapterGetOAuthConfig(provider);
        if (config.register_allowed) {
          return true;
        }
      } catch {
        // fall through
      }

      return `${routes.guest.login}?error=OAuthNotLinked`;
    },
    async jwt({ token, user, account, trigger, session }) {
      if (user) {
        token.sub = user.id;
        token.email = user.email;
        const extended = user as typeof user & {
          accessToken?: string;
          refreshToken?: string;
          expiresIn?: number;
        };
        if (extended.accessToken && extended.refreshToken) {
          token.accessToken = extended.accessToken;
          token.refreshToken = extended.refreshToken;
          token.expiresIn = extended.expiresIn;
        }
      }

      if (
        account &&
        (account.provider === "passkey" || isOAuthProvider(account.provider)) &&
        token.sub &&
        !token.accessToken
      ) {
        try {
          const authMethod =
            account.provider === "passkey" ? "passkey" : "oauth";
          const tokens = await adapterIssueSession(token.sub, authMethod);
          token.accessToken = tokens.access_token;
          token.refreshToken = tokens.refresh_token;
          token.expiresIn = tokens.expires_in;
        } catch {
          token.error = isOAuthProvider(account.provider)
            ? "OAuthSessionError"
            : "PasskeySessionError";
        }
      }

      if (trigger === "update" && session) {
        const patch = session as {
          accessToken?: string;
          refreshToken?: string;
          expiresIn?: number;
        };
        if (patch.accessToken) token.accessToken = patch.accessToken;
        if (patch.refreshToken) token.refreshToken = patch.refreshToken;
        if (patch.expiresIn) token.expiresIn = patch.expiresIn;
      }

      return token;
    },
    async session({ session, token }) {
      if (session.user) {
        session.user.id = token.sub ?? "";
      }
      (session as { error?: string }).error = token.error as
        "PasskeySessionError" | "OAuthSessionError" | undefined;
      return session;
    },
  },
}));

export const { handlers, auth, signIn, signOut } = authHandlers;
