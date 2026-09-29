import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §21 Iteration 5: the Ask UI (§10) and the admin logs (§15.4). The stack
// runs with AI off, so the real questions take the keyword path; the answered
// path is a canned stream in the shape §11.6 defines.
test("a member asks from Home and the node page, and an admin reads the Ask and audit logs", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `A${run.slice(-6)}`;
  const password = "e2e-ask-admin-passphrase-9";

  await setPassword(page, process.env.E2E_ASK_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_ASK_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
  const project = await call("POST", "/projects", { key, name: `Ask ${run}` });
  const hr = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  const overtime = await call("POST", `/projects/${key}/nodes`, { type: "menu", name: "Overtime Approval", parent_id: hr.id });
  const title = `Overtime approval skips the supervisor ${run}`;
  const ticket = await call("POST", `/projects/${key}/tickets`, {
    type: "change_request", title, node_ids: [overtime.id], reason: "Supervisors are often on leave; HR approves overtime directly.",
  });
  // The index job runs in the background; the keyword path reads its chunks.
  await expect
    .poll(async () => (await call("POST", "/ask", { question: `supervisor ${run}`, scope: { project_ids: [project.id] } })).results.map((r: { key: string }) => r.key), {
      intervals: [1000, 2000, 3000, 5000],
      timeout: 30_000,
    })
    .toContain(ticket.key);

  // AC-IX-5 from the Home Ask box: AI is off, so the answer is keyword results.
  await page.goto("/");
  await page.getByRole("search", { name: "Tanya" }).getByLabel("Pertanyaan").fill(`Kenapa supervisor dilewati ${run}?`);
  await page.getByRole("search", { name: "Tanya" }).getByRole("button", { name: "Tanya" }).click();
  await expect(page).toHaveURL(/\/ask/);
  const offAnswer = page.getByRole("article", { name: `Kenapa supervisor dilewati ${run}?` });
  await expect(offAnswer.getByText("AI dimatikan.")).toBeVisible();
  await expect(offAnswer.getByRole("link", { name: ticket.key })).toBeVisible();
  await expect(page.getByRole("complementary", { name: "Percakapan" }).getByRole("link", { name: `Kenapa supervisor dilewati ${run}?` })).toBeVisible();

  // The answered path (§10.3), streamed from a canned response: claims with
  // citation chips, sources, and a detected chip that one click removes (§10.2).
  const asked: { ignore?: unknown }[] = [];
  await page.route("**/api/v1/ask", async (route) => {
    asked.push(route.request().postDataJSON());
    const item = { key: ticket.key, title, client: null, requested_by: "Ask Admin", date: new Date().toISOString(), status: "To do", closed: false };
    const events = [
      ["scope", { explicit: { node_ids: [overtime.id] }, detected: { user_ids: [7], labels: [{ kind: "user", id: 7, label: "Budi Santoso" }] } }],
      ["evidence", [item]],
      ["claim", { text: "HR approves overtime directly when supervisors are on leave.", cites: [ticket.key] }],
      ["result", { status: "answered", query_id: 1, thread_id: 999999, language: "en", model: "Local · qwen3.5:4b", closest: [], results: [] }],
    ];
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
      body: events.map(([name, data]) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`).join(""),
    });
  });
  await page.goto(`/p/${key}/modules/${overtime.id}?tab=ask`);
  const box = page.getByRole("form", { name: "Tanya" });
  await expect(box.getByText("HR › Overtime Approval")).toBeVisible(); // the node chip comes preset
  await box.getByLabel("Pertanyaan").fill("Why is the supervisor skipped, requested by Budi?");
  await box.getByLabel("Pertanyaan").press("Enter");
  const answer = page.getByRole("article", { name: "Why is the supervisor skipped, requested by Budi?" }).first();
  await expect(answer.getByText("Dijawab dari 1 sumber")).toBeVisible();
  const chip = answer.getByRole("list").first().getByRole("link", { name: ticket.key }); // the claim's chip, not the source row
  await expect(chip).toHaveAttribute("target", "_blank");
  await chip.hover();
  await expect(answer.getByRole("tooltip")).toContainText(title);
  await expect(answer.getByRole("heading", { name: "Sumber" })).toBeVisible();
  await expect(answer.getByText("Local · qwen3.5:4b")).toBeVisible();
  await expect(answer.getByText("terdeteksi")).toBeVisible();
  await answer.getByRole("button", { name: "Hapus Budi Santoso" }).click();
  await expect.poll(() => asked.length).toBe(2);
  expect(asked[1].ignore).toEqual([{ kind: "user", id: 7 }]);
  await page.unroute("**/api/v1/ask");

  // §15.4: the admin finds the real question in the Ask log and opens it.
  await page.goto("/admin/ask-log?status=ai_off");
  await page.getByRole("link", { name: `Kenapa supervisor dilewati ${run}?` }).click();
  await expect(page.getByRole("heading", { name: `Kenapa supervisor dilewati ${run}?` })).toBeVisible();
  await expect(page.getByText("AI mati")).toBeVisible();

  // §15.4: the audit log, filtered, and its CSV export.
  await page.goto("/admin/audit?entity=project");
  await expect(page.getByRole("row").filter({ hasText: key }).first()).toContainText("create");
  const exported = await page.request.get(`/api/v1/admin/audit/export?entity=project`);
  expect(exported.headers()["content-type"]).toContain("text/csv");
  expect((await exported.text()).split("\n")[0]).toBe("id,occurred_at,actor,via,entity,entity_id,project,action,changes");
});
