import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getMe, serverApi } from "@/lib/server-api";
import UsersAdmin from "./UsersAdmin";

export default async function UsersPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("users");
  const { data } = me.is_admin ? await (await serverApi()).GET("/admin/users") : { data: undefined };
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
      </PageBar>
      <main className="mx-auto max-w-6xl p-4 md:p-5">
        {me.is_admin ? <UsersAdmin users={data?.items ?? []} meId={me.id} /> : <p className="text-muted">{t("adminsOnly")}</p>}
      </main>
    </>
  );
}
