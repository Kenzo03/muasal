import { getTranslations } from "next-intl/server";
import SetupForm from "./SetupForm";

export default async function SetupPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const t = await getTranslations("setup");
  return (
    <main className="mx-auto mt-24 max-w-sm rounded-lg border bg-white p-8 shadow-sm">
      <h1 className="mb-6 text-xl font-semibold">{t("title")}</h1>
      <SetupForm token={token} />
    </main>
  );
}
