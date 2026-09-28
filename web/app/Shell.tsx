import { cookies } from "next/headers";
import { getMe, getProjects } from "@/lib/server-api";
import Frame from "./Frame";

// The frame of every signed-in page (FSD §6.1): the sidebar, the top bar, then
// the page. Sign-in and setup get the page alone.
export default async function Shell({ children }: { children: React.ReactNode }) {
  const me = await getMe();
  if (!me) return children;
  const [projects, store] = await Promise.all([getProjects(), cookies()]);
  return (
    <Frame me={me} projects={projects} rail={store.get("nav")?.value === "rail"}>
      {children}
    </Frame>
  );
}
