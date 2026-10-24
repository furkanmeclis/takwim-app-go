"use client";

import { useEffect, useState } from "react";

/**
 * Reads a link token from the URL fragment (`/panel/l#token`) once and
 * removes it from the address bar and the history entry. The fragment
 * never reaches a server, so no log holds the token; the page sends it to
 * the API in a request body. `undefined` until read, `""` when absent.
 */
export function useFragmentToken(): string | undefined {
  const [token, setToken] = useState<string | undefined>(undefined);
  useEffect(() => {
    const raw = window.location.hash.replace(/^#/, "").trim();
    if (raw) {
      window.history.replaceState(
        window.history.state,
        "",
        window.location.pathname + window.location.search,
      );
    }
    // Reading the address is a one-time sync from outside React.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setToken(raw);
  }, []);
  return token;
}
