import createClient, { type Middleware } from "openapi-fetch";

import { apiConfig } from "@/config/api";
import type { paths } from "@/generated/api";

import { ApiError, emitApiError, parseApiError } from "./errors";
import { runStepUpEnsure } from "@/features/step-up-engine/lib/step-up-interceptor";

type Envelope<T> = {
  success?: boolean;
  data?: T;
  meta?: { request_id?: string };
};

let refreshPromise: Promise<boolean> | null = null;
let onAuthFailure: (() => void | Promise<void>) | null = null;

export function setAuthFailureHandler(
  handler: (() => void | Promise<void>) | null,
) {
  onAuthFailure = handler;
}

async function refreshSession(): Promise<boolean> {
  const response = await fetch(`${apiConfig.baseUrl}/v1/auth/refresh`, {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify({}),
  });

  if (!response.ok) return false;

  const json = (await response.json()) as Envelope<{ authenticated?: boolean }>;
  return Boolean(json.data?.authenticated ?? json.success);
}

function enqueueRefresh() {
  if (!refreshPromise) {
    refreshPromise = refreshSession().finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise;
}

const bffMiddleware: Middleware = {
  async onRequest({ request }) {
    const headers = new Headers(request.headers);
    if (!headers.has("Accept")) {
      headers.set("Accept", "application/json");
    }
    return new Request(request, {
      headers,
      credentials: "include",
    });
  },

  async onResponse({ request, response }) {
    if (response.status === 403) {
      const payload = (await response
        .clone()
        .json()
        .catch(() => null)) as { error?: { code?: string } } | null;
      if (payload?.error?.code === "STEP_UP_REQUIRED") {
        const ok = await runStepUpEnsure();
        if (ok) {
          return fetch(
            new Request(request, {
              headers: request.headers,
              credentials: "include",
            }),
          );
        }
      }
    }

    if (response.status !== 401) {
      return response;
    }

    const url = new URL(request.url);
    if (
      url.pathname.endsWith("/auth/login") ||
      url.pathname.endsWith("/auth/refresh") ||
      url.pathname.endsWith("/auth/logout")
    ) {
      return response;
    }

    const ok = await enqueueRefresh();
    if (!ok) {
      await onAuthFailure?.();
      return response;
    }

    return fetch(
      new Request(request, {
        headers: request.headers,
        credentials: "include",
      }),
    );
  },
};

export const apiClient = createClient<paths>({
  baseUrl: apiConfig.baseUrl,
  credentials: "include",
});

apiClient.use(bffMiddleware);

type FetchLikeResult = {
  data?: unknown;
  error?: unknown;
  response: Response;
};

export async function unwrap<T>(
  result: FetchLikeResult,
  options?: { silent?: boolean },
): Promise<T> {
  const { data, error, response } = result;

  if (!response.ok || error) {
    const apiError = parseApiError(response.status, error ?? data);
    if (!options?.silent && apiError.code !== "STEP_UP_REQUIRED") {
      emitApiError(apiError);
    }
    throw apiError;
  }

  const envelope = data as Envelope<T> | T | undefined;
  if (
    envelope &&
    typeof envelope === "object" &&
    "data" in envelope &&
    (envelope as Envelope<T>).data !== undefined
  ) {
    return (envelope as Envelope<T>).data as T;
  }

  return data as T;
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

export async function fetchSession(): Promise<{
  authenticated: boolean;
  has_access: boolean;
  has_refresh: boolean;
  user_id: string | null;
}> {
  const response = await fetch("/api/auth/session", {
    method: "GET",
    credentials: "include",
    headers: { Accept: "application/json" },
    cache: "no-store",
  });

  if (!response.ok) {
    return {
      authenticated: false,
      has_access: false,
      has_refresh: false,
      user_id: null,
    };
  }

  const session = (await response.json()) as {
    user?: { id?: string };
    error?: string;
  };

  const authenticated = Boolean(session.user && !session.error);

  return {
    authenticated,
    has_access: authenticated,
    has_refresh: authenticated,
    user_id: session.user?.id ?? null,
  };
}
