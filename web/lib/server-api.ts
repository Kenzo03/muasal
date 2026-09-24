import createClient from "openapi-fetch";
import { cookies } from "next/headers";
import type { paths } from "./api-types";

// Server Components read through the Go API on the internal network and
// forward the session cookie. Next.js never writes data (FSD §3.1).
export async function serverApi() {
  const store = await cookies();
  const sid = store.get("sid")?.value;
  return createClient<paths>({
    baseUrl: `${process.env.API_INTERNAL_URL ?? "http://localhost:8080"}/api/v1`,
    headers: sid ? { cookie: `sid=${sid}` } : {},
  });
}

/** The signed-in user, or undefined when the session is missing or has ended. */
export async function getMe() {
  const { data } = await (await serverApi()).GET("/me");
  return data;
}
