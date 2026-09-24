import { getRequestConfig } from "next-intl/server";
import { cookies } from "next/headers";

// The UI language comes from the `locale` cookie, which the app sets from the
// user's profile at sign-in (FSD §6.3). Indonesian is the default.
export default getRequestConfig(async () => {
  const store = await cookies();
  const locale = store.get("locale")?.value === "en" ? "en" : "id";
  return { locale, messages: (await import(`../messages/${locale}.json`)).default };
});
