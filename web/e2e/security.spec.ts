import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §18.2: pages carry a nonce-based Content-Security-Policy and the other
// security headers, and nothing the app runs trips the policy.
test("pages send the security headers and run under the policy", async ({ page }) => {
  const violations: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error" && /Content Security Policy|Refused to/.test(m.text())) violations.push(m.text());
  });

  const login = await page.goto("/login");
  const h = login!.headers();
  expect(h["content-security-policy"]).toMatch(/script-src 'self' 'nonce-[A-Za-z0-9+/=]+' 'strict-dynamic'/);
  expect(h["content-security-policy"]).toContain("frame-ancestors 'none'");
  expect(h["x-content-type-options"]).toBe("nosniff");
  expect(h["referrer-policy"]).toBe("same-origin");
  expect(h["strict-transport-security"]).toBe("max-age=31536000");
  const api = await page.request.get("/api/v1/me");
  expect(api.headers()["content-security-policy"]).toBe("default-src 'none'; frame-ancestors 'none'");

  // Sign-in, Home and the Ask panel all need the page's scripts.
  const password = "e2e-security-admin-passphrase-4";
  await setPassword(page, process.env.E2E_SECURITY_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_SECURITY_ADMIN_EMAIL!, password);
  await page.getByRole("button", { name: "Tanya" }).first().click();
  await expect(page.getByRole("dialog", { name: "Tanya" })).toBeVisible();
  await page.goto("/admin/audit");
  await expect(page.getByRole("heading", { name: "Log audit" })).toBeVisible();
  expect(violations).toEqual([]);
});
