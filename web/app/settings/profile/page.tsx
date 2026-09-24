import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import ProfileForm from "./ProfileForm";

export default async function ProfilePage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("profile");
  return (
    <main className="mx-auto max-w-md p-8">
      <Link className="text-sm underline" href="/">{t("back")}</Link>
      <h1 className="mb-6 mt-2 text-2xl font-semibold">{t("title")}</h1>
      <ProfileForm me={me} />
    </main>
  );
}
