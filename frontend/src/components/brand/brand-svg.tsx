import type { SVGProps } from "react";

import type { BrandTone } from "@/config/brand";
import { cn } from "@/lib/utils";

import { TAKWIM_PATHS } from "./takwim-paths";

type BrandSvgProps = Omit<SVGProps<SVGSVGElement>, "children"> & {
  title: string;
  tone?: BrandTone;
};

/**
 * Fill classes per tone. `tone` names the background the art sits on:
 * `light` → colourful kit (navy mark, sky dot, ink letters), `dark` → white
 * kit, `auto` → colourful in light mode, white in dark mode.
 */
const MARK_FILL: Record<BrandTone, string> = {
  auto: "fill-[#022C81] dark:fill-white",
  light: "fill-[#022C81]",
  dark: "fill-white",
};
const DOT_FILL: Record<BrandTone, string> = {
  auto: "fill-[#60A7FE] dark:fill-white",
  light: "fill-[#60A7FE]",
  dark: "fill-white",
};
const WORD_FILL: Record<BrandTone, string> = {
  auto: "fill-[#041F47] dark:fill-white",
  light: "fill-[#041F47]",
  dark: "fill-white",
};

function Svg({
  viewBox,
  title,
  className,
  children,
  ...props
}: Omit<BrandSvgProps, "tone"> & {
  viewBox: string;
  children: React.ReactNode;
}) {
  return (
    <svg
      viewBox={viewBox}
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("shrink-0", className)}
      {...props}
    >
      <title>{title}</title>
      {children}
    </svg>
  );
}

function MarkPaths({
  paths,
  className,
}: {
  paths: readonly { d: string; transform: string }[];
  className: string;
}) {
  return paths.map((p) => (
    <path
      key={p.d.slice(0, 24)}
      className={className}
      fillRule="evenodd"
      {...p}
    />
  ));
}

/** Horizontal lockup: calendar mark + "Takwim" letters. */
export function TakwimLogoSvg({ tone = "auto", ...props }: BrandSvgProps) {
  const { mark, dot, word } = TAKWIM_PATHS.logo;
  return (
    <Svg viewBox="58 73 1885 483" {...props}>
      <MarkPaths paths={mark} className={MARK_FILL[tone]} />
      <circle className={DOT_FILL[tone]} {...dot} />
      <path className={WORD_FILL[tone]} fillRule="evenodd" {...word} />
    </Svg>
  );
}

/** Stacked lockup: mark above the letters. */
export function TakwimStackedSvg({ tone = "auto", ...props }: BrandSvgProps) {
  const { mark, dot, word } = TAKWIM_PATHS.stacked;
  return (
    <Svg viewBox="262 501 1476 950" {...props}>
      <MarkPaths paths={mark} className={MARK_FILL[tone]} />
      <path className={DOT_FILL[tone]} fillRule="evenodd" {...dot} />
      <path className={WORD_FILL[tone]} fillRule="evenodd" {...word} />
    </Svg>
  );
}

/** Letters only ("Takwim"). */
export function TakwimWordSvg({ tone = "auto", ...props }: BrandSvgProps) {
  return (
    <Svg viewBox="612 214 1331 287" {...props}>
      <path
        className={WORD_FILL[tone]}
        fillRule="evenodd"
        {...TAKWIM_PATHS.logo.word}
      />
    </Svg>
  );
}

/** The calendar mark alone, square frame. */
export function TakwimMarkSvg({ tone = "auto", ...props }: BrandSvgProps) {
  const { mark, dot } = TAKWIM_PATHS.mark;
  return (
    <Svg viewBox="583 543 834 834" {...props}>
      <MarkPaths paths={mark} className={MARK_FILL[tone]} />
      <circle className={DOT_FILL[tone]} {...dot} />
    </Svg>
  );
}

/**
 * App icon: white calendar and sky dot on the navy square. Same art on any
 * background, so it takes no tone.
 */
export function TakwimIconSvg(props: Omit<BrandSvgProps, "tone">) {
  const { mark, dot } = TAKWIM_PATHS.mark;
  return (
    <Svg viewBox="0 0 2000 2000" {...props}>
      <rect width="2000" height="2000" fill="#022C81" />
      <MarkPaths paths={mark} className="fill-white" />
      <circle fill="#60A7FE" {...dot} />
    </Svg>
  );
}
