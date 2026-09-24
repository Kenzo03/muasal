import createClient from "openapi-fetch";
import type { paths } from "./api-types";

// Browser client: same origin, so the session cookie and the Origin header go along by themselves.
export const api = createClient<paths>({ baseUrl: "/api/v1" });
