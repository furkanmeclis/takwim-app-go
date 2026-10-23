import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

export default function ProfilePreferencesPage() {
  redirect(routes.account.profile.root);
}
