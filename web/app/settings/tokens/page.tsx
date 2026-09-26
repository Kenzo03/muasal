import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getMe, serverApi } from "@/lib/server-api";
import Tokens from "./Tokens";

// Settings → API tokens (FSD §14.3).
export default async function TokensPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("tokens");
  const { data } = await (await serverApi()).GET("/me/tokens");
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
      </PageBar>
      <main className="p-4 md:p-5">
        <Tokens tokens={data?.items ?? []} />
      </main>
    </>
  );
}
