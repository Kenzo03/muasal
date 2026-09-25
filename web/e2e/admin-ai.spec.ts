import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §13.4 on real screens: a fresh install runs with AI off, Bring your own
// key is refused without the acknowledgement (AC-IX-6), and Off saves.
test("an admin keeps AI off and cannot send data out without acknowledging it", async ({ page }) => {
  const email = process.env.E2E_AI_ADMIN_EMAIL!;
  const password = "Ai-admin-password-2026!";
  await setPassword(page, process.env.E2E_AI_ADMIN_LINK!, password);
  await signIn(page, email, password);

  await page.getByRole("link", { name: "AI", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "AI" })).toBeVisible();
  await expect(page.getByRole("radio", { name: "Mati" })).toBeChecked();
  await expect(page.getByRole("heading", { name: "Status indeks" })).toBeVisible();

  await page.getByRole("radio", { name: "Pakai kunci sendiri" }).check();
  await page.getByLabel("Penyedia", { exact: true }).fill("Contoh Cloud");
  await expect(page.getByText("Saya memahami bahwa pertanyaan dan kutipan tiket akan dikirim ke Contoh Cloud.")).toBeVisible();
  await page.getByRole("button", { name: "Simpan" }).click();
  await expect(page.getByText("Konfirmasi bahwa pertanyaan dan kutipan tiket akan dikirim ke penyedia")).toBeVisible();

  await page.getByRole("radio", { name: "Mati" }).check();
  await page.getByRole("button", { name: "Simpan" }).click();
  await expect(page.getByRole("status")).toHaveText("Tersimpan. Pertanyaan berikutnya memakai pengaturan ini.");
  await page.reload();
  await expect(page.getByRole("radio", { name: "Mati" })).toBeChecked();
});
