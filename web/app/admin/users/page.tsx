import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe, serverApi } from "@/lib/server-api";
import UsersAdmin from "./UsersAdmin";

export default async function UsersPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("users");
  if (!me.is_admin) {
    return <main className="p-8">{t("adminsOnly")}</main>;
  }
  const { data } = await (await serverApi()).GET("/admin/users");
  return (
    <main className="mx-auto max-w-5xl p-8">
      <Link className="text-sm underline" href="/">{t("back")}</Link>
      <h1 className="mb-6 mt-2 text-2xl font-semibold">{t("title")}</h1>
      <UsersAdmin users={data?.items ?? []} meId={me.id} />
    </main>
  );
}
