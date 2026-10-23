import {
  PLATFORM_SCOPE,
  scopedRequest,
  type ApiScope,
} from "@/lib/api/scoped-request";

import type {
  RemoteSearchSpec,
  SearchHit,
} from "@/features/search-engine/types";

type SearchSpecsResponse = {
  items: RemoteSearchSpec[];
  enabled: boolean;
};

type SearchHitsResponse = {
  items: SearchHit[];
};

/**
 * Inside an organization the palette searches `/v1/tenant/search` (records of
 * that organization only); elsewhere the platform `/v1/search`.
 */
function searchBase(scope: ApiScope) {
  return scope.kind === "tenant" ? "/v1/tenant/search" : "/v1/search";
}

export async function fetchSearchSpecs(
  scope: ApiScope = PLATFORM_SCOPE,
): Promise<SearchSpecsResponse> {
  return scopedRequest<SearchSpecsResponse>(
    scope,
    "GET",
    `${searchBase(scope)}/specs`,
  );
}

export async function fetchSearchHits(
  q: string,
  spec?: string,
  limit = 20,
  scope: ApiScope = PLATFORM_SCOPE,
): Promise<SearchHit[]> {
  const data = await scopedRequest<SearchHitsResponse>(
    scope,
    "GET",
    searchBase(scope),
    { query: { q, spec, limit } },
  );
  return data.items ?? [];
}
