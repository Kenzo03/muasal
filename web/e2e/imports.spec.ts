import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §14.2: a system admin imports a Jira export: dry run, a value map fixes
// the rejected row, the background job runs, and the old key finds the ticket
// (R-IN-2). Running the same file again changes no counts (AC-IN-3).
test("an admin imports a Jira export twice", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `J${run.slice(-6)}`;
  const password = "e2e-import-admin-passphrase-6";

  await setPassword(page, process.env.E2E_IMPORT_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_IMPORT_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  await call("POST", "/projects", { key, name: `Jira ${run}` });
  await call("POST", `/projects/${key}/nodes`, { type: "menu", name: "Overtime Approval" });
  const jira = [
    "Summary,Issue key,Issue Type,Status,Reporter,Created,Resolved,Component/s,Comment",
    `Overtime export ${run},OLD-${run}-1,Story,Done,Budi,12/Mar/24 2:05 PM,20/Mar/24 9:00 AM,Overtime Approval,14/Mar/24 10:00 AM;Budi;Format confirmed.`,
    `Spike ${run},OLD-${run}-2,Spike,To Do,Budi,13/Mar/24 9:00 AM,,,`,
  ].join("\n");

  const importOnce = async (fix: boolean) => {
    await page.goto("/admin/imports");
    const form = page.getByRole("form", { name: "Impor baru" });
    await form.getByLabel("Proyek").selectOption(key);
    await form.getByLabel("Berkas").setInputFiles({ name: "jira.csv", mimeType: "text/csv", buffer: Buffer.from(jira) });
    await form.getByRole("button", { name: "Unggah dan uji coba" }).click();
    await expect(page).toHaveURL(/\/admin\/imports\/\d+$/);
    const dry = page.getByRole("region", { name: "Uji coba" });
    if (fix) {
      await expect(dry.getByText("Ditolak")).toBeVisible();
      await expect(page.getByRole("region", { name: "Kesalahan pertama" }).getByText(/Unknown type "Spike"/)).toBeVisible();
      const values = page.getByLabel("Peta nilai");
      await values.fill(`${await values.inputValue()}\ntypes: spike = change_request`);
      await page.getByRole("button", { name: "Uji coba lagi" }).click();
    }
    await page.getByRole("button", { name: "Impor 2 tiket" }).click();
    await expect(page.getByRole("status")).toHaveText(/Selesai: 2 tiket ditulis/, { timeout: 30_000 });
  };
  await importOnce(true);
  const found = await call("GET", `/search?q=OLD-${run}-1`);
  expect(found.tickets.map((t: { title: string }) => t.title)).toEqual([`Overtime export ${run}`]);
  const count = async () => (await call("GET", `/projects/${key}/tickets`)).items.length;
  expect(await count()).toBe(2);

  // AC-IN-3: the same file, mapped the same way, updates and adds nothing.
  await importOnce(true);
  expect(await count()).toBe(2);
  await expect(page.getByRole("status")).toHaveText(/0 komentar baru/);
});
