import {
  Activity,
  Armchair,
  CalendarClock,
  ConciergeBell,
  UserRound,
  Bell,
  Boxes,
  Contact,
  Download,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  Package,
  ScrollText,
  Settings2,
  Shield,
  Store,
  Upload,
  Users,
  Wallet,
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
  branches: Store,
  staff: Contact,
  resources: Armchair,
  services: ConciergeBell,
  bundles: Boxes,
  customers: UserRound,
  reservations: CalendarClock,
  cash: Wallet,
  packages: Package,
};

export function resolveSearchIcon(name?: string): LucideIcon {
  if (!name) return LayoutDashboard;
  return ICONS[name.toLowerCase()] ?? LayoutDashboard;
}
