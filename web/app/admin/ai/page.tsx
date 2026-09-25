import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getMe, serverApi } from "@/lib/server-api";
import AIAdmin from "./AIAdmin";

// Admin → AI (FSD §13.4): the AI mode, the model endpoints and Index status.
export default async function AIPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("ai");
  const api = await serverApi();
  const [settings, status] = me.is_admin
    ? await Promise.all([api.GET("/admin/settings/ai"), api.GET("/admin/ai/status")])
    : [undefined, undefined];
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        {settings?.data && <span className="text-[13px] text-muted">{settings.data.mode === "off" ? t("offBadge") : settings.data.badge}</span>}
      </PageBar>
      <main className="mx-auto max-w-5xl p-4 md:p-5">
        {settings?.data && status?.data ? (
          <AIAdmin settings={settings.data} status={status.data} />
        ) : (
          <p className="text-muted">{t("adminsOnly")}</p>
        )}
      </main>
    </>
  );
}
