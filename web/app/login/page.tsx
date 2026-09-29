import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import AuthCard from "@/components/AuthCard";
import { getMe } from "@/lib/server-api";
import LoginForm from "./LoginForm";

export default async function LoginPage() {
  // Already signed in, as in a tab left here that reloads: go home. Switching
  // accounts starts with signing out.
  if (await getMe()) redirect("/");
  const t = await getTranslations("login");
  return (
    <AuthCard title={t("title")}>
      <LoginForm />
    </AuthCard>
  );
}
