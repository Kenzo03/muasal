import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe, serverApi } from "@/lib/server-api";
import ClientsAdmin from "./ClientsAdmin";

export default async function ClientsPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("clients");
  if (!me.is_admin) return <main className="p-8">{t("adminsOnly")}</main>;
  const { data } = await (await serverApi()).GET("/clients");
  return (
    <main className="mx-auto max-w-5xl p-8">
      <h1 className="mb-6 text-2xl font-semibold">{t("title")}</h1>
      <ClientsAdmin clients={data?.items ?? []} />
    </main>
  );
}
