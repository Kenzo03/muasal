import { expect, type Locator, type Page } from "@playwright/test";

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

// Drags with several pointer moves, as dnd-kit waits for a short move before a
// drag starts. The grab point sits near the top-left corner, clear of links and
// menus inside the source; `to` picks the drop point as a share of the target's box.
export async function drag(page: Page, source: Locator, target: Locator, to = { x: 0.5, y: 0.5 }) {
  const from = await source.boundingBox();
  const box = await target.boundingBox();
  if (!from || !box) throw new Error("drag: source or target is not visible");
  await page.mouse.move(from.x + 8, from.y + 8);
  await page.mouse.down();
  await page.mouse.move(from.x + 20, from.y + 20, { steps: 4 });
  await page.mouse.move(box.x + box.width * to.x, box.y + box.height * to.y, { steps: 12 });
  await page.mouse.up();
  // dnd-kit stops every click for 50 ms after a drop, so a click on what the drop
  // opens (the close dialog's Batal) can be lost on a fast server. Wait it out.
  await page.waitForTimeout(100);
}
