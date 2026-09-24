import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import SignOutButton from "./SignOutButton";

export default async function Home() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("home");
  return (
    <main className="mx-auto max-w-2xl p-8">
      <h1 className="text-2xl font-semibold">Muasal</h1>
      <p className="mt-2">{t("signedInAs", { name: me.name })}</p>
      <nav className="mt-6 flex gap-4">
        <Link className="underline" href="/settings/profile">{t("profile")}</Link>
        {me.is_admin && <Link className="underline" href="/admin/users">{t("users")}</Link>}
        <SignOutButton label={t("signOut")} />
      </nav>
    </main>
  );
}
