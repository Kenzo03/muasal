import { expect, type Page } from "@playwright/test";

// The tests run in the default UI language, Indonesian: after sign-in the UI
// follows the user's profile language, and new users start with `id`.

export async function setPassword(page: Page, link: string, password: string) {
  await page.goto(link);
  await page.getByLabel("Kata sandi baru").fill(password);
  await page.getByLabel("Ulangi kata sandi").fill(password);
  await page.getByRole("button", { name: "Simpan kata sandi" }).click();
  await expect(page.getByRole("status")).toHaveText("Kata sandi tersimpan. Silakan masuk.");
}

export async function signIn(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Kata sandi").fill(password);
  await page.getByRole("button", { name: "Masuk" }).click();
  await expect(page).toHaveURL(/\/$/);
}
