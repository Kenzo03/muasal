import { getProject, serverApi } from "@/lib/server-api";

// What the create form needs, for the full page and the modal alike.
export async function newTicketData(key: string) {
  const project = await getProject(key);
  if (!project) return null;
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
  return {
    project,
    clients: clients.data?.items ?? [],
    nodes: nodes.data?.items ?? [],
    assignees: assignees.data?.items ?? [],
  };
}
