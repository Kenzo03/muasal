import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §21 Iteration 2 exit check: a PM logs a client request in one form (story 3),
// then works it: an Internal comment (AC-TK-10) and a move on the board (AC-TK-1).
test("a PM logs a client request in one form and moves it on the board", async ({ page, browser }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase(); // keys and names stay unique across runs on one stack
  const key = `T${run.slice(-6)}`;
  const clientName = `Klien A ${run}`;
  const pmEmail = `rina-${run.toLowerCase()}@example.com`;
  const adminPassword = "e2e-ticket-admin-passphrase-5";
  const pmPassword = "e2e-rina-pm-passphrase-6";

  // The admin sets the project up through the API; the screens for that have their own test.
  await setPassword(page, process.env.E2E_TICKET_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_TICKET_ADMIN_EMAIL!, adminPassword);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  const client = await call("POST", "/clients", { name: clientName });
  await call("POST", "/projects", { key, name: `HRIS ${run}` });
  await call("PUT", `/projects/${key}/clients`, { client_ids: [client.id] });
  const hr = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  await call("POST", `/projects/${key}/nodes`, {
    type: "menu", name: "Overtime Approval", parent_id: hr.id, client_specific: true, client_ids: [client.id],
  });
  const pm = await call("POST", "/admin/users", { name: "Rina PM", email: pmEmail });
  await call("PUT", `/projects/${key}/members`, { members: [{ email: pmEmail, role: "member", all_clients: true }] });

  // Story 3: while Budi is on the phone, Rina logs his request in one form.
  const rina = await (await browser.newContext()).newPage();
  await setPassword(rina, pm.setup_link.url, pmPassword);
  await signIn(rina, pmEmail, pmPassword);
  await rina.goto(`/p/${key}/tickets/new`);
  const form = rina.getByRole("form", { name: "Tiket baru" });
  await form.getByLabel("Klien").selectOption({ label: clientName });
  await form.getByRole("radio", { name: "Seorang kontak" }).check();
  await form.getByRole("button", { name: "+ Tambah kontak" }).click();
  await form.getByLabel("Nama kontak").fill("Budi");
  await form.getByLabel("Jabatan (opsional)").fill("HR Manager");
  await form.getByRole("button", { name: "Tambah", exact: true }).click();
  await expect(form.getByLabel(/^Kontak/)).not.toHaveValue(""); // the label text also holds the options
  await form.getByLabel("Judul").fill("Skip supervisor approval for overtime");
  await form.getByRole("checkbox", { name: "HR › Overtime Approval" }).check();
  await form.getByLabel("Alasan").fill("Client A supervisors are often on leave; HR approves overtime directly.");
  await form.getByRole("button", { name: "Buat", exact: true }).click();

  await expect(rina).toHaveURL(new RegExp(`/t/${key}-1$`));
  await expect(rina.getByRole("heading", { name: "Skip supervisor approval for overtime" })).toBeVisible();
  await expect(rina.getByText("Budi (HR Manager)")).toBeVisible();
  await expect(rina.getByRole("listitem").filter({ hasText: /^Overtime Approval$/ })).toBeVisible();
  await expect(rina.getByText("Tambahkan alasan dan minimal satu menu sebelum menutup.")).toHaveCount(0);

  // AC-TK-10: a new comment is Internal by default.
  await rina.getByLabel("Tulis komentar").fill("Budi confirmed by phone.");
  await rina.getByRole("button", { name: "Kirim" }).click();
  const comment = rina.getByRole("listitem").filter({ hasText: "Budi confirmed by phone." });
  await expect(comment.getByText("Internal", { exact: true })).toBeVisible();

  // AC-TK-1: dragging the card from To do to In progress records the move.
  await rina.goto(`/p/${key}/board`);
  const inProgress = rina.getByRole("region", { name: "In progress" });
  await rina.getByRole("article").filter({ hasText: `${key}-1` }).dragTo(inProgress);
  await expect(inProgress.getByRole("article")).toContainText(`${key}-1`);
  await rina.goto(`/t/${key}-1`);
  await expect(rina.getByText(/Rina PM mengubah Status dari To do menjadi In progress/)).toBeVisible();
});
