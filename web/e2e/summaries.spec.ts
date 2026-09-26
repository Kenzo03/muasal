import { expect, test, type Page } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

function api(page: Page) {
  return async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: new URL(page.url()).origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
}

// FSD §9.3 (AC-DC-6, AC-DC-7) and §12.1: "Draft with AI" fills the close
// dialog without saving, leaves Why empty and highlighted when the thread
// never says why, and keeps what the user typed. The summary builder then
// previews the closed ticket by menu. The model is stubbed at the browser's
// edge; the Go tests cover the real model calls.
test("AI drafts a decision record and the builder previews a summary", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `S${run.slice(-6)}`;
  const password = "e2e-summary-admin-passphrase-4";
  await setPassword(page, process.env.E2E_SUMMARY_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_SUMMARY_ADMIN_EMAIL!, password);
  const call = api(page);
  const client = await call("POST", "/clients", { name: `Sentosa ${run}` });
  await call("POST", "/projects", { key, name: `Payroll ${run}` });
  await call("PUT", `/projects/${key}/clients`, { client_ids: [client.id] });
  const payroll = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "Payroll" });
  const tk = await call("POST", `/projects/${key}/tickets`, {
    type: "change_request", title: "Round overtime to the nearest 15 minutes", client_id: client.id, node_ids: [payroll.id],
    reason: "Asked in the monthly review.",
  });

  let drafts = 0;
  await page.route(`**/api/v1/tickets/${tk.key}/decision-draft`, async (route) => {
    drafts++;
    await route.fulfill({
      json: { what_changed: "Overtime is rounded to the nearest 15 minutes.", why: "", alternatives: "Rounding to the hour.", model: "Local · qwen3.5:9b" },
    });
  });
  await page.goto(`/t/${tk.key}`);
  await page.getByLabel(/^Status/).selectOption({ label: "Done" });
  const dialog = page.getByRole("dialog", { name: `Tutup ${tk.key} sebagai Done` });
  await dialog.getByLabel("Alternatif yang ditolak").fill("Typed by hand.");
  await dialog.getByRole("button", { name: "Draf dengan AI" }).click();
  await expect(dialog.getByLabel("Apa yang berubah")).toHaveValue("Overtime is rounded to the nearest 15 minutes.");
  await expect(dialog.getByLabel("Mengapa")).toHaveValue("");
  await expect(dialog.getByText("Percakapan tiket tidak menyebut alasannya")).toBeVisible();
  await expect(dialog.getByLabel("Alternatif yang ditolak")).toHaveValue("Typed by hand.");
  await expect(dialog.getByRole("button", { name: "Tutup tiket" })).toBeDisabled();
  expect(drafts).toBe(1);
  let ticket = await call("GET", `/tickets/${tk.key}`);
  expect(ticket.decision).toBeUndefined(); // nothing saved until Close ticket

  await dialog.getByLabel("Mengapa").fill("Sentosa's payroll policy pays overtime in quarter hours.");
  await dialog.getByRole("button", { name: "Tutup tiket" }).click();
  await expect(page.getByRole("region", { name: "Catatan keputusan" })).toContainText("Overtime is rounded to the nearest 15 minutes.");
  ticket = await call("GET", `/tickets/${tk.key}`);
  expect(ticket.decision.ai_drafted).toBe(true);

  // The builder lists the closed ticket under its menu, ticked, ready to generate.
  await page.goto(`/p/${key}/summaries`);
  await page.getByRole("link", { name: "Ringkasan baru" }).click();
  await page.getByRole("combobox", { name: /^Klien/ }).selectOption({ label: `Sentosa ${run}` });
  await page.getByRole("button", { name: "Tampilkan item" }).click();
  const items = page.getByRole("region", { name: "1 dari 1 item dicentang" });
  await expect(items.getByRole("group", { name: "Payroll" })).toContainText(tk.key);
  await expect(items.getByRole("checkbox")).toBeChecked();
  await expect(items.getByRole("button", { name: "Buat ringkasan" })).toBeVisible();
});
