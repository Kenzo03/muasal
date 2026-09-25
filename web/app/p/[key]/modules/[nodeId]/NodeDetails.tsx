"use client";

import { useRouter } from "next/navigation";
import type { Client, Node } from "@/lib/problem";
import { cx, panel } from "@/lib/ui";
import NodeForm, { ReadOnlyNode } from "../NodeForm";

// The Details tab (FSD §7.4): project admins edit the node here; everyone else
// reads it. Archived nodes are read-only until restored in the tree (R-MR-4).
export default function NodeDetails({ projectKey, node, clients, canEdit }: { projectKey: string; node: Node; clients: Client[]; canEdit: boolean }) {
  const router = useRouter();
  return (
    <div className={cx(panel, "max-w-3xl p-4")}>
      {canEdit && !node.archived ? (
        <NodeForm projectKey={projectKey} node={node} clients={clients} onSaved={() => router.refresh()} />
      ) : (
        <ReadOnlyNode node={node} />
      )}
    </div>
  );
}
