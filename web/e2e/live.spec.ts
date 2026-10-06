import { expect, request, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// Spec: live ticket updates. An agent's change (an API token, as MCP uses)
// shows on an open board and ticket page without a reload.
test("an open board and ticket page follow changes made elsewhere", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `L${run.slice(-6)}`;
  const password = "e2e-live-admin-passphrase-7";
  await setPassword(page, process.env.E2E_LIVE_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_LIVE_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  await call("POST", "/projects", { key, name: `Live ${run}` });
  const node = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "Live" });
  const ticket = await call("POST", `/projects/${key}/tickets`, { type: "bug", title: "Watched ticket", node_ids: [node.id] });
  const statuses = (await call("GET", `/projects/${key}/statuses`)).items as { id: number; name: string }[];
  const inProgress = statuses.find((s) => s.name === "In progress")!.id;
  const { token } = await call("POST", "/me/tokens", { name: "Live agent", read_only: false });
  const agent = await request.newContext({ baseURL: origin, extraHTTPHeaders: { Authorization: `Bearer ${token}` } });

  // The board: the agent moves the ticket; the card changes column, no reload.
  await page.goto(`/p/${key}/board`);
  const column = page.getByRole("region", { name: "In progress" });
  await expect(column.getByRole("article")).toHaveCount(0);
  const moved = await agent.post(`/api/v1/tickets/${ticket.key}/transition`, { data: { status_id: inProgress } });
  expect(moved.ok(), `transition: ${moved.status()}`).toBeTruthy();
  await expect(column.getByRole("article")).toContainText(ticket.key, { timeout: 5_000 });

  // The ticket page, editing: the bar appears; cancelling shows the change.
  await page.goto(`/t/${ticket.key}`);
  await page.getByRole("button", { name: "Ubah", exact: true }).first().click();
  // PUT replaces every editable field; this ticket only has a requester besides these.
  const current = await agent.get(`/api/v1/tickets/${ticket.key}`);
  const { requester } = (await current.json()) as { requester: { kind: string; id: number } };
  const renamed = await agent.put(`/api/v1/tickets/${ticket.key}`, {
    data: {
      type: "bug", title: "Renamed by the agent", node_ids: [node.id],
      [requester.kind === "user" ? "requester_user_id" : "requester_contact_id"]: requester.id,
    },
    headers: { "If-Match": current.headers()["etag"] },
  });
  expect(renamed.ok(), `rename: ${renamed.status()}`).toBeTruthy();
  await expect(page.getByText("Tiket ini diperbarui.")).toBeVisible({ timeout: 5_000 });
  await page.getByRole("button", { name: "Batal" }).click();
  await expect(page.getByText("Tiket ini diperbarui.")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Renamed by the agent" })).toBeVisible();
  await agent.dispose();
});
