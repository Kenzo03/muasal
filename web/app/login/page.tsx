import { getTranslations } from "next-intl/server";
import AuthCard from "@/components/AuthCard";
import LoginForm from "./LoginForm";

export default async function LoginPage() {
  const t = await getTranslations("login");
  return (
    <AuthCard title={t("title")}>
      <LoginForm />
    </AuthCard>
  );
}
