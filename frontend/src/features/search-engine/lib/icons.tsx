import {
  Activity,
  Bell,
  Download,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  ScrollText,
  Settings2,
  Shield,
  Upload,
  Users,
  type LucideIcon,
} from "lucide-react";

import { githubNavIcon } from "@/components/icons/github-icon";

const ICONS: Record<string, LucideIcon> = {
  users: Users,
  shield: Shield,
  pages: LayoutDashboard,
  bell: Bell,
  activity: Activity,
  logs: ScrollText,
  storage: HardDrive,
  access: KeyRound,
  imports: Upload,
  exports: Download,
  settings: Settings2,
  home: LayoutDashboard,
  github: githubNavIcon,
};

export function resolveSearchIcon(name?: string): LucideIcon {
  if (!name) return LayoutDashboard;
  return ICONS[name.toLowerCase()] ?? LayoutDashboard;
}
