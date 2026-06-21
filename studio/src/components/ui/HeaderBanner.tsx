// HeaderBanner is the thin sub-toolbar strip used under page/run headers
// to surface a single line of context (source ticket, fork breadcrumb,
// etc). It encapsulates the shared `shrink-0 px-4 py-1.5 border-b flex
// items-center gap-2 text-[11px]` skeleton + per-tone soft/border tint,
// while accepting className overrides for site-specific tweaks.
import type { HTMLAttributes } from "react";

export type HeaderBannerTone = "info" | "warning" | "danger" | "success";

export interface HeaderBannerProps extends HTMLAttributes<HTMLDivElement> {
  tone: HeaderBannerTone;
}

const toneClass: Record<HeaderBannerTone, string> = {
  info: "bg-info-soft/40 border-info/30",
  warning: "bg-warning-soft/40 border-warning/30",
  danger: "bg-danger-soft/40 border-danger/30",
  success: "bg-success-soft/40 border-success/30",
};

export function HeaderBanner({
  tone,
  className = "",
  children,
  ...rest
}: HeaderBannerProps) {
  const base =
    "shrink-0 px-4 py-1.5 border-b flex items-center gap-2 text-[11px]";
  return (
    <div className={`${base} ${toneClass[tone]} ${className}`.trim()} {...rest}>
      {children}
    </div>
  );
}
