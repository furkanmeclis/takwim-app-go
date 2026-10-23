import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

export default function ProfilePasswordPage() {
  redirect(routes.account.profile.root);
}
