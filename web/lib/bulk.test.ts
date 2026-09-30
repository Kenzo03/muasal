import assert from "node:assert/strict";
import test from "node:test";
import { updateBody } from "./bulk.ts";

const ticket = {
  type: "bug", title: "Cut-off payroll", client: { id: 3, name: "PT Sinar Retail" }, requester: { kind: "contact", id: 9, name: "Lina" },
  nodes: [{ id: 30, name: "Perhitungan Gaji" }], reason: "Gaji salah", description: "", assignee: { id: 3, name: "Fajar" },
  priority: "urgent", due_date: "2026-10-01", estimate_hours: 6.5, labels: ["pilot"], release: { id: 4, name: "v1.0" },
} as unknown as Parameters<typeof updateBody>[0];

// MSL-53: a bulk change keeps everything it doesn't name.
test("keeps what the change leaves out", () => {
  const body = updateBody(ticket, { assignee: "2" });
  assert.deepEqual(body, {
    type: "bug", title: "Cut-off payroll", client_id: 3, requester_contact_id: 9, requester_user_id: undefined, node_ids: [30],
    reason: "Gaji salah", description: "", assignee_id: 2, priority: "urgent", due_date: "2026-10-01", estimate_hours: 6.5, labels: ["pilot"],
    release_id: 4,
  });
  assert.equal(updateBody(ticket, { release: "7" }).release_id, 7); // MSL-67
  assert.equal(updateBody(ticket, { release: "" }).release_id, undefined);
  assert.equal(updateBody(ticket, { assignee: "", due: "" }).assignee_id, undefined);
  assert.equal(updateBody(ticket, { assignee: "", due: "" }).due_date, undefined);
  assert.equal(updateBody(ticket, { priority: "low", due: "2026-10-09" }).due_date, "2026-10-09");
});
