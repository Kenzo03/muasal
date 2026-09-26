import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §18.3 and §15.6: an admin reads System status, then runs a backup from
// the Backups page and sees it listed; the stack's backup service makes it.
test("an admin checks the system and runs a backup", async ({ page }) => {
  test.setTimeout(120_000);
  const password = "e2e-ops-admin-passphrase-3";
  await setPassword(page, process.env.E2E_OPS_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_OPS_ADMIN_EMAIL!, password);

  await page.goto("/admin/system");
  await expect(page.getByRole("heading", { name: "Status sistem" })).toBeVisible();
  await expect(page.getByText("Basis data")).toBeVisible();
  await expect(page.getByText("AI dimatikan")).toBeVisible();
  await expect(page.getByRole("meter", { name: "Lampiran" })).toBeVisible();
  await expect(page.getByRole("meter", { name: "Cadangan" })).toBeVisible();

  await page.goto("/admin/backups");
  await page.getByRole("button", { name: "Cadangkan sekarang" }).click();
  await expect(page.getByRole("status")).toHaveText("Diminta; pencadangan dimulai dalam 30 detik.");
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/admin/backups")).json()).items.length, { intervals: [5000], timeout: 90_000 })
    .toBeGreaterThan(0);
  await page.reload();
  await expect(page.getByRole("cell", { name: /^db-\d{8}-\d{4}\.dump$/ }).first()).toBeVisible();
});
