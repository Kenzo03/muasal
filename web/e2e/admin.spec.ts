import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// The admin Users and Clients screens: edit in the side panel, confirm before
// a reset, search, and archive a client from the row menu.
test("an admin edits a user, confirms a reset, searches, and archives a client", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36);
  const password = "e2e-admin-screens-passphrase-7";
  await setPassword(page, process.env.E2E_ADMIN2_LINK!, password);
  await signIn(page, process.env.E2E_ADMIN2_EMAIL!, password);

  // A user to work on.
  await page.getByRole("link", { name: "Pengguna" }).click();
  await page.getByRole("button", { name: "Pengguna baru" }).click();
  let panel = page.getByRole("dialog", { name: "Pengguna baru" });
  await panel.getByLabel("Nama", { exact: true }).fill(`Sari ${run}`);
  await panel.getByLabel("Email", { exact: true }).fill(`sari-${run}@example.com`);
  await panel.getByRole("button", { name: "Buat pengguna" }).click();
  await expect(page.getByTestId("setup-link").first()).toContainText("/setup/");

  // Search narrows the table to her.
  await page.getByLabel("Nama atau email").fill(`sari-${run}`);
  await expect(page.locator("tbody tr")).toHaveCount(1);

  // Rename her and make her an admin from the panel.
  await page.getByRole("button", { name: `Sari ${run}` }).click();
  panel = page.getByRole("dialog", { name: `Sari ${run}` });
  await panel.getByLabel("Nama", { exact: true }).fill(`Sari Dewi ${run}`);
  await panel.getByLabel("Admin sistem").check();
  await panel.getByRole("button", { name: "Simpan" }).click();
  await expect(panel).toBeHidden();
  await page.getByLabel("Nama atau email").fill(`Sari Dewi ${run}`);
  const row = page.locator("tbody tr").filter({ hasText: `Sari Dewi ${run}` });
  await expect(row.getByText("Admin", { exact: true })).toBeVisible();

  // A new setup link asks first; cancelling changes nothing.
  await row.getByLabel(`Tindakan untuk Sari Dewi ${run}`).click();
  await page.getByRole("button", { name: "Tautan pengaturan baru" }).click();
  const confirm = page.getByRole("alertdialog");
  await expect(confirm).toContainText(`Sari Dewi ${run}`);
  await confirm.getByRole("button", { name: "Batal" }).click();
  await expect(confirm).toBeHidden();

  // An unlinked client shows "—" for its project; archiving asks, then moves it to Archived.
  const client = `Klien ${run}`;
  await page.getByRole("link", { name: "Klien", exact: true }).click();
  await page.getByRole("button", { name: "Klien baru" }).click();
  panel = page.getByRole("dialog", { name: "Klien baru" });
  await panel.getByLabel("Nama", { exact: true }).fill(client);
  await panel.getByLabel("Alias").fill("KR Group");
  await panel.getByLabel("Alias").press("Enter");
  await expect(panel.getByRole("button", { name: "Hapus alias KR Group" })).toBeVisible();
  await panel.getByRole("button", { name: "Buat klien" }).click();
  await expect(panel).toBeHidden();
  await page.getByLabel("Nama, kode, atau alias").fill("kr group");
  const crow = page.locator("tbody tr").filter({ hasText: client });
  await expect(crow).toContainText("—");
  await crow.getByLabel(`Tindakan untuk ${client}`).click();
  await page.getByRole("button", { name: "Arsipkan", exact: true }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Arsipkan" }).click();
  await expect(crow).toBeHidden();
  await page.getByRole("button", { name: /^Diarsipkan/ }).click();
  await expect(page.locator("tbody tr").filter({ hasText: client })).toBeVisible();
});
