"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import {
  EMBED_BASE,
  PAGE_BASE,
  embedQuery,
  postToParent,
  resolveParentOrigin,
  type EmbedContext,
} from "@/features/business-page/lib/embed";

/**
 * Where the booking screen runs: the business page (`/b/{slug}`, channel
 * `web`) or the embedded screen (`/embed/{slug}`, channel `widget`). Both
 * use the same wizard and status page (requirements §7: the full and the
 * compact view share one booking infrastructure).
 */
export type BookingSurface = {
  channel: "web" | "widget";
  /** Base path of the status page. */
  base: string;
  /** Query the embedded screen keeps across its pages. */
  query: string;
  embed: EmbedContext | null;
  /** Brings the top of the screen into view after a step change. */
  scrollTop: () => void;
};

const pageSurface: BookingSurface = {
  channel: "web",
  base: PAGE_BASE,
  query: "",
  embed: null,
  scrollTop: () => window.scrollTo({ top: 0, behavior: "smooth" }),
};

const SurfaceContext = createContext<BookingSurface>(pageSurface);

export function useBookingSurface() {
  return useContext(SurfaceContext);
}

/**
 * Frame side of the embedded screen. It measures its content and tells the
 * loader the height (inline frames grow with the wizard), asks the parent
 * to scroll the frame into view after a step change, and announces itself
 * once. Messages go only to the parent's checked origin and carry no
 * customer data.
 */
export function EmbedFrame({
  ctx,
  allowedOrigins,
  children,
}: {
  ctx: EmbedContext;
  allowedOrigins: string[];
  children: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [target, setTarget] = useState<string | null>(null);
  const originsKey = allowedOrigins.join(" ");

  useEffect(() => {
    const origin = resolveParentOrigin(ctx, originsKey.split(" "));
    // The parent is known only in the browser; resolve it once per list.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTarget(origin);
    postToParent(origin, ctx.frame, "ready");
  }, [ctx, originsKey]);

  useEffect(() => {
    const node = ref.current;
    if (!node || !target || ctx.modal) return;
    let last = 0;
    let frame = 0;
    const send = () => {
      frame = 0;
      const height = Math.ceil(node.getBoundingClientRect().height);
      if (height !== last) {
        last = height;
        postToParent(target, ctx.frame, "resize", { height });
      }
    };
    const observer = new ResizeObserver(() => {
      if (!frame) frame = window.requestAnimationFrame(send);
    });
    observer.observe(node);
    send();
    return () => {
      observer.disconnect();
      if (frame) window.cancelAnimationFrame(frame);
    };
  }, [target, ctx.frame, ctx.modal]);

  const surface = useMemo<BookingSurface>(
    () => ({
      channel: "widget",
      base: EMBED_BASE,
      query: embedQuery(ctx),
      embed: ctx,
      scrollTop: () => {
        if (ctx.modal || !target) {
          window.scrollTo({ top: 0, behavior: "smooth" });
        } else {
          postToParent(target, ctx.frame, "scroll_top");
        }
      },
    }),
    [ctx, target],
  );

  return (
    <SurfaceContext.Provider value={surface}>
      <div ref={ref} className={ctx.modal ? "min-h-svh" : undefined}>
        {children}
      </div>
    </SurfaceContext.Provider>
  );
}
