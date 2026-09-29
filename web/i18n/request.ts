import { getRequestConfig } from "next-intl/server";
import { cookies } from "next/headers";
import { getMe } from "@/lib/server-api";

// The UI language comes from the `locale` cookie, which the app sets from the
// user's profile at sign-in (FSD §6.3). Indonesian is the default.
// Times show in the profile's timezone; pages read it with getTimeZone() or
// useTimeZone(). getMe is cached per request, so this adds no API call.
export default getRequestConfig(async () => {
  const store = await cookies();
  const locale = store.get("locale")?.value === "en" ? "en" : "id";
  const me = await getMe();
  return { locale, timeZone: known(me?.timezone), messages: (await import(`../messages/${locale}.json`)).default };
});

// A zone this runtime lacks would break every date on every page, so it falls back to UTC.
function known(timeZone?: string) {
  if (!timeZone) return "UTC";
  try {
    new Intl.DateTimeFormat("en-US", { timeZone });
    return timeZone;
  } catch {
    return "UTC";
  }
}
