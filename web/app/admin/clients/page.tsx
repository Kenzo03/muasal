import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getMe, serverApi } from "@/lib/server-api";
import ClientsAdmin from "./ClientsAdmin";

export default async function ClientsPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("clients");
  const { data } = me.is_admin ? await (await serverApi()).GET("/clients") : { data: undefined };
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        {me.is_admin && <span className="inline-flex h-6 items-center rounded-full bg-[#EFE9E2] px-2.5 text-[12.5px] font-extrabold text-ink-soft">{data?.items.length ?? 0}</span>}
      </PageBar>
      <main className="mx-auto max-w-6xl p-4 md:p-5">
        {me.is_admin ? <ClientsAdmin clients={data?.items ?? []} /> : <p className="text-muted">{t("adminsOnly")}</p>}
      </main>
    </>
  );
}
