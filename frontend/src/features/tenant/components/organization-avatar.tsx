"use client";

import { Building2 } from "lucide-react";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { cn } from "@/lib/utils";

/**
 * Organization logo from the API-owned `logo_url` (never an object key),
 * falling back to a building icon.
 */
export function OrganizationAvatar({
  logoUrl,
  className,
}: {
  logoUrl?: string | null;
  className?: string;
}) {
  const src = resolveLogoSrc(assertServiceMediaURL(logoUrl));
  return (
    <Avatar className={cn("size-8 rounded-lg border bg-white", className)}>
      {src ? (
        <AvatarImage src={src} alt="" className="object-contain p-0.5" />
      ) : null}
      <AvatarFallback className="bg-sidebar-primary text-sidebar-primary-foreground rounded-lg">
        <Building2 className="size-4" />
      </AvatarFallback>
    </Avatar>
  );
}
