import { getTranslations } from "next-intl/server";
import AuthCard from "@/components/AuthCard";
import SetupForm from "./SetupForm";

export default async function SetupPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const t = await getTranslations("setup");
  return (
    <AuthCard title={t("title")}>
      <SetupForm token={token} />
    </AuthCard>
  );
}
