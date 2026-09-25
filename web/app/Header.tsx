import { getMe, getProjects } from "@/lib/server-api";
import TopBar from "./TopBar";

// The top bar on every signed-in page (FSD §6.1). Ask joins it in Iteration 5.
export default async function Header() {
  const me = await getMe();
  if (!me) return null;
  return <TopBar me={me} projects={await getProjects()} />;
}
