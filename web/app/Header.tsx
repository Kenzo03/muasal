import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import SignOutButton from "./SignOutButton";

// The top bar on every signed-in page (FSD §6.1). Search, Ask and New ticket join it in later iterations.
export default async function Header() {
  const me = await getMe();
  if (!me) return null;
  const t = await getTranslations("nav");
  return (
    <header className="border-b bg-white">
      <nav aria-label={t("label")} className="mx-auto flex max-w-6xl items-center gap-5 px-6 py-3 text-sm">
        <Link href="/" className="font-semibold">Muasal</Link>
        {me.is_admin && <Link href="/admin/users">{t("users")}</Link>}
        {me.is_admin && <Link href="/admin/clients">{t("clients")}</Link>}
        <span className="ml-auto" />
        <Link href="/settings/profile">{t("profile")}</Link>
        <SignOutButton label={t("signOut")} />
      </nav>
    </header>
  );
}
