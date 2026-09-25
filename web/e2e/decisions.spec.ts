import { expect, test, type Page } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// api returns a JSON caller that acts as the page's signed-in user.
function api(page: Page) {
  return async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: new URL(page.url()).origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
}

// FSD §21 Iteration 3 exit check: a developer sees a menu's history newest
// first (story 1). On the way, closes from the ticket page and from the board
// ask for the decision record (AC-DC-1, AC-DC-2, AC-DC-4, AC-TK-2), Home shows
// the developer's ticket, and search finds the menu and jumps to a key.
test("a developer sees a menu's history newest first", async ({ page, browser }) => {
  test.setTimeout(180_000);
  const run = Date.now().toString(36).toUpperCase(); // keys and names stay unique across runs on one stack
  const key = `D${run.slice(-6)}`;
  const clientName = `Arunika ${run}`;
  const rinaEmail = `rina-${run.toLowerCase()}@example.com`;
  const dimasEmail = `dimas-${run.toLowerCase()}@example.com`;
  const adminPassword = "e2e-decision-admin-passphrase-7";
  const rinaPassword = "e2e-rina-decision-passphrase-8";
  const dimasPassword = "e2e-dimas-dev-passphrase-9";

  // The admin sets the project up through the API; its screens have their own tests.
  await setPassword(page, process.env.E2E_DECISION_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_DECISION_ADMIN_EMAIL!, adminPassword);
  const asAdmin = api(page);
  const client = await asAdmin("POST", "/clients", { name: clientName });
  await asAdmin("POST", "/projects", { key, name: `HRIS ${run}` });
  await asAdmin("PUT", `/projects/${key}/clients`, { client_ids: [client.id] });
  const hr = await asAdmin("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  const attendance = await asAdmin("POST", `/projects/${key}/nodes`, { type: "module", name: "Attendance", parent_id: hr.id });
  const overtime = await asAdmin("POST", `/projects/${key}/nodes`, { type: "menu", name: "Overtime Approval", parent_id: attendance.id });
  const rina = await asAdmin("POST", "/admin/users", { name: "Rina PM", email: rinaEmail });
  const dimas = await asAdmin("POST", "/admin/users", { name: "Dimas Dev", email: dimasEmail });
  await asAdmin("PUT", `/projects/${key}/members`, {
    members: [
      { email: rinaEmail, role: "member", all_clients: true },
      { email: dimasEmail, role: "member", all_clients: true },
    ],
  });

  // Rina files three requests on Overtime Approval; the second has no reason yet.
  const pm = await (await browser.newContext()).newPage();
  await setPassword(pm, rina.setup_link.url, rinaPassword);
  await signIn(pm, rinaEmail, rinaPassword);
  const asRina = api(pm);
  const file = (title: string, fields: object) =>
    asRina("POST", `/projects/${key}/tickets`, { type: "change_request", title, client_id: client.id, node_ids: [overtime.id], ...fields });
  const inThreeDays = new Date(Date.now() + 3 * 86_400_000).toISOString().slice(0, 10);
  await file("Overtime cap of 40 hours a month", { reason: "Arunika's labor agreement caps overtime at 40 hours a month." });
  await file("Weekend overtime at double rate", {});
  await file("Skip supervisor approval for overtime", {
    reason: "Supervisors are often on leave; HR approves overtime directly.",
    assignee_id: dimas.user.id,
    due_date: inThreeDays,
  });

  // AC-DC-1 and AC-DC-2: closing from the ticket page asks for the decision record, prefilled.
  await pm.goto(`/t/${key}-1`);
  await pm.getByLabel(/^Status/).selectOption({ label: "Done" });
  const dialog = pm.getByRole("dialog", { name: `Tutup ${key}-1 sebagai Done` });
  const whatChanged = dialog.getByLabel("Apa yang berubah");
  await expect(whatChanged).toHaveValue("Overtime cap of 40 hours a month");
  await expect(dialog.getByLabel("Mengapa")).toHaveValue("Arunika's labor agreement caps overtime at 40 hours a month.");
  await whatChanged.fill("");
  await expect(dialog.getByRole("button", { name: "Tutup tiket" })).toBeDisabled();
  await expect(dialog.getByText("Wajib diisi")).toBeVisible();
  await whatChanged.fill("Payroll blocks overtime approvals above 40 hours a month.");
  await dialog.getByLabel("Alternatif yang ditolak").fill("A warning without a block, rejected because payroll still paid the hours.");
  await dialog.getByRole("button", { name: "Tutup tiket" }).click();
  const record = pm.getByRole("region", { name: "Catatan keputusan" });
  await expect(record).toContainText("Dikonfirmasi oleh Rina PM");
  await expect(record).toContainText("Payroll blocks overtime approvals above 40 hours a month.");

  // AC-TK-2: dropping a card on Done opens the dialog, and Batal leaves the card in To do.
  await pm.goto(`/p/${key}/board`);
  const todo = pm.getByRole("region", { name: "To do" });
  const done = pm.getByRole("region", { name: "Done" });
  const weekend = pm.getByRole("article").filter({ hasText: `${key}-2` });
  await weekend.dragTo(done);
  const dialog2 = pm.getByRole("dialog", { name: `Tutup ${key}-2 sebagai Done` });
  await dialog2.getByRole("button", { name: "Batal" }).click();
  await expect(dialog2).toHaveCount(0);
  await expect(todo).toContainText(`${key}-2`);

  // AC-DC-4: the ticket has no reason, so the dialog asks for one and flags a weak one.
  await weekend.dragTo(done);
  const reason = dialog2.getByLabel("Alasan");
  await reason.fill("sesuai permintaan klien");
  await expect(dialog2.getByText(/Jelaskan mengapa klien membutuhkannya/)).toBeVisible();
  await reason.fill("Arunika's new labor agreement pays weekend overtime at double rate.");
  await dialog2.getByLabel("Apa yang berubah").fill("Weekend overtime pays double for Arunika from October.");
  await dialog2.getByLabel("Mengapa").fill("The new labor agreement doubles weekend overtime pay.");
  await dialog2.getByRole("button", { name: "Tutup tiket" }).click();
  await expect(done).toContainText(`${key}-2`);

  // Dimas, a developer, starts from Home, where the assigned ticket waits under Tiket saya.
  const dev = await (await browser.newContext()).newPage();
  await setPassword(dev, dimas.setup_link.url, dimasPassword);
  await signIn(dev, dimasEmail, dimasPassword);
  await expect(dev.getByRole("region", { name: "Tiket saya" }).getByRole("link", { name: `${key}-3` })).toBeVisible();
  await expect(dev.getByRole("region", { name: "Baru diperbarui" })).toContainText(`${key}-2`);

  // Story 1: search finds the menu, and its page lists the history newest first.
  const search = dev.getByRole("searchbox", { name: "Cari tiket atau menu" });
  await search.fill("Overtime Appr");
  await search.press("Enter");
  await dev.getByRole("link", { name: /HR › Attendance › Overtime Approval/ }).click();
  await expect(dev.getByRole("heading", { level: 1, name: "Overtime Approval" })).toBeVisible();
  const open = dev.getByRole("region", { name: /^Sedang berjalan/ });
  await expect(open.getByRole("article")).toHaveCount(1);
  await expect(open).toContainText(`${key}-3`);
  const history = dev.getByRole("region", { name: /^Ditutup/ }).getByRole("article");
  await expect(history).toHaveCount(2);
  await expect(history.nth(0)).toContainText(`${key}-2`); // closed last, so listed first
  await expect(history.nth(0)).toContainText("Weekend overtime pays double for Arunika from October.");
  await expect(history.nth(0)).toContainText("The new labor agreement doubles weekend overtime pay.");
  await expect(history.nth(1)).toContainText(`${key}-1`);
  await expect(history.nth(1)).toContainText("Payroll blocks overtime approvals above 40 hours a month.");

  // Story 5: Behaviors by client lists the decisions in force under the client.
  await dev.getByRole("link", { name: "Perilaku per klien" }).click();
  await expect(dev.getByRole("region", { name: clientName })).toContainText(`${key}-1`);

  // A key typed in the search box opens its ticket (FSD §6.1).
  await search.fill(`${key.toLowerCase()}-1`);
  await search.press("Enter");
  await expect(dev).toHaveURL(new RegExp(`/t/${key}-1$`));
});
