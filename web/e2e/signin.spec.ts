import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

const adminPassword = "e2e-admin-passphrase-1";
const userPassword = "e2e-budi-passphrase-2";

// FSD §21 Iteration 0 exit check: a user signs in on the compose stack.
test("an admin creates a user who sets a password and signs in", async ({ page, browser }) => {
  await setPassword(page, process.env.E2E_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_ADMIN_EMAIL!, adminPassword);
  await expect(page.getByText("Masuk sebagai E2E Admin")).toBeVisible();

  await page.getByRole("link", { name: "Pengguna" }).click();
  const email = `budi-${Date.now()}@example.com`;
  await page.getByLabel("Nama").fill("Budi");
  await page.getByLabel("Email").fill(email);
  await page.getByRole("button", { name: "Buat pengguna" }).click();
  const link = await page.getByTestId("setup-link").textContent();
  expect(link).toContain("/setup/");

  const budi = await (await browser.newContext()).newPage();
  await setPassword(budi, link!, userPassword);
  await signIn(budi, email, userPassword);
  await expect(budi.getByText("Masuk sebagai Budi")).toBeVisible();
});
