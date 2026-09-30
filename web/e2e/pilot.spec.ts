import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD P1 part one: ticket links that supersede a decision (§8.8, AC-TK-8),
// decision notes on the node timeline (§9.4, AC-DC-8) and thumbs-down
// feedback that reaches the Ask log (§10.7, AC-AK-8).
test("a member reverses a decision, records a meeting's decision, and rates an answer", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `P${run.slice(-6)}`;
  const password = "e2e-pilot-admin-passphrase-4";

  await setPassword(page, process.env.E2E_PILOT_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_PILOT_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
  await call("POST", "/projects", { key, name: `Pilot ${run}` });
  const hr = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  const overtime = await call("POST", `/projects/${key}/nodes`, { type: "menu", name: "Overtime Approval", parent_id: hr.id });
  const reason = "Supervisors are often on leave; HR approves overtime directly.";
  const old = await call("POST", `/projects/${key}/tickets`, { type: "change_request", title: "Skip supervisor approval", node_ids: [overtime.id], reason });
  const statuses = await call("GET", `/projects/${key}/statuses`);
  const done = statuses.items.find((s: { name: string }) => s.name === "Done");
  await call("POST", `/tickets/${old.key}/transition`, {
    status_id: done.id, reason, node_ids: [overtime.id],
    decision: { what_changed: "Overtime approval skips the supervisor.", why: "Approvals stalled during leave periods.", alternatives: "" },
  });
  const now = await call("POST", `/projects/${key}/tickets`, { type: "change_request", title: "Bring back supervisor approval", node_ids: [overtime.id], reason: "Audit findings." });

  // AC-TK-8: the new ticket reverses the old one, whose decision is then superseded.
  await page.goto(`/t/${now.key}`);
  const links = page.getByRole("region", { name: "Tautan" });
  await links.getByRole("button", { name: "Tambah tautan" }).click();
  await links.getByLabel("Tiket ini").selectOption({ label: "Mencabut" });
  await links.getByLabel("Kunci tiket").fill(old.key);
  await links.getByRole("button", { name: "Tautkan" }).click();
  await expect(links.getByRole("link", { name: old.key })).toBeVisible();
  await expect(links.getByText("Mencabut", { exact: true })).toBeVisible();
  await page.goto(`/t/${old.key}`);
  await expect(page.getByRole("region", { name: "Tautan" }).getByText("Dicabut oleh")).toBeVisible();
  await expect(page.getByRole("region", { name: "Catatan keputusan" }).getByText(`Digantikan oleh ${now.key}`)).toBeVisible();

  // AC-DC-8: a note from a meeting, dated 2025-11-04, on Overtime Approval.
  await page.goto(`/p/${key}/notes`);
  await page.getByRole("link", { name: "Notulen baru" }).click();
  const form = page.getByRole("form", { name: "Notulen keputusan baru" });
  await form.getByLabel("Judul").fill(`Overtime meeting ${run}`);
  await form.getByLabel("Tanggal keputusan").fill("2025-11-04");
  await form.getByLabel("Peserta").fill("Hana, Budi (HR Manager)");
  await form.getByRole("checkbox", { name: "HR › Overtime Approval" }).check();
  await form.getByLabel("Tiket terkait").fill(old.key);
  await form.getByRole("button", { name: "Simpan" }).click();
  await expect(page).toHaveURL(new RegExp(`/notes/${key}-DN1$`));
  await expect(page.getByRole("heading", { level: 1, name: `Overtime meeting ${run}` })).toBeVisible();
  await page.goto(`/p/${key}/modules/${overtime.id}`);
  const note = page.getByRole("article", { name: `${key}-DN1 Overtime meeting ${run}` });
  await expect(note).toBeVisible();
  await expect(note.getByText("Bersama Hana, Budi (HR Manager)")).toBeVisible();
  await expect(page.getByRole("article", { name: `${old.key} Skip supervisor approval` }).getByText(`Digantikan oleh ${now.key}`)).toBeVisible();

  // AC-AK-8: a thumbs-down with "Kutipan salah" shows in the Ask log's filter.
  // AI is off, so the answered stream is canned around a real, logged question.
  const question = `Kenapa supervisor dilewati ${run}?`;
  const logged = await call("POST", "/ask", { question });
  await page.route("**/api/v1/ask", async (route) => {
    const item = { kind: "note", key: `${key}-DN1`, title: `Overtime meeting ${run}`, client: null, requested_by: "Pilot Admin", date: "2025-11-04T00:00:00Z", status: "", closed: true };
    const events = [
      ["scope", { explicit: {}, detected: {} }],
      ["evidence", [item]],
      ["claim", { text: "HR approves overtime directly.", cites: [item.key] }],
      ["result", { status: "answered", query_id: logged.query_id, thread_id: logged.thread_id, language: "id", model: "Local · qwen3.5:4b", closest: [], results: [] }],
    ];
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
      body: events.map(([e, d]) => `event: ${e}\ndata: ${JSON.stringify(d)}\n\n`).join(""),
    });
  });
  await page.goto("/ask");
  await page.getByLabel("Pertanyaan").fill(question);
  await page.getByRole("form", { name: "Tanya" }).getByRole("button", { name: "Tanya" }).click();
  const answer = page.getByRole("article", { name: question });
  await expect(answer.getByRole("link", { name: `${key}-DN1` }).first()).toHaveAttribute("href", `/notes/${key}-DN1`);
  await answer.getByRole("button", { name: /Tidak/ }).click();
  const why = answer.getByRole("form", { name: "Apa yang salah?" });
  await why.getByRole("checkbox", { name: "Kutipan salah" }).check();
  await why.getByRole("button", { name: "Kirim" }).click();
  await expect(answer.getByText("Terima kasih, sudah dicatat.")).toBeVisible();
  await page.unroute("**/api/v1/ask");

  await page.goto("/admin/ask-log");
  await page.getByRole("link", { name: "Jempol ke bawah" }).click();
  const row = page.getByRole("row", { name: new RegExp(question.replace(/[?]/g, "\\?")) });
  await expect(row).toBeVisible();
  await expect(row.getByText("👎 Kutipan salah")).toBeVisible();
});
