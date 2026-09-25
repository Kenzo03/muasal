import type { Node } from "./problem";

/** Each node's path from the top of the tree, "HR › Attendance › Overtime Approval"; "" for a node not in the list. */
export function nodePaths(nodes: Node[]): (id: number) => string {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  return (id) => {
    const names: string[] = [];
    for (let n = byId.get(id); n; n = n.parent_id === null ? undefined : byId.get(n.parent_id)) names.unshift(n.name);
    return names.join(" › ");
  };
}
