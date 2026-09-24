import { getMe, getProjects } from "@/lib/server-api";
import TopBar from "./TopBar";

// The top bar on every signed-in page (FSD §6.1). Search and Ask join it in later iterations.
export default async function Header() {
  const me = await getMe();
  if (!me) return null;
  return <TopBar me={me} projects={await getProjects()} />;
}
