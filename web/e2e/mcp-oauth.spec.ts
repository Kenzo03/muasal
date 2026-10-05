import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// MCP spec: an agent registers, the user signs in and allows it on the
// approval page, and the browser returns to the agent with a code.
test("an AI agent is allowed, then denied, through the approval page", async ({ page }) => {
  const password = "e2e-mcp-admin-passphrase-8";
  const redirect = "http://127.0.0.1:9/callback";
  const reg = await page.request.post("/oauth/register", { data: { client_name: "E2E Agent", redirect_uris: [redirect] } });
  expect(reg.status()).toBe(201);
  const { client_id } = await reg.json();
  const authorize = `/oauth/authorize?${new URLSearchParams({
    client_id, redirect_uri: redirect, response_type: "code", state: "s1",
    code_challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", code_challenge_method: "S256",
  })}`;

  await setPassword(page, process.env.E2E_MCP_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_MCP_ADMIN_EMAIL!, password);

  await page.goto(authorize);
  await expect(page.getByText("E2E Agent ingin memakai Zettra atas nama Anda.")).toBeVisible();
  await expect(page.getByText("127.0.0.1:9")).toBeVisible();
  // The redirect target has no server; catch the navigation instead of loading it.
  await page.route(`${redirect}**`, (route) => route.fulfill({ status: 200, body: "ok" }));
  await page.getByRole("button", { name: "Izinkan" }).click();
  await page.waitForURL(`${redirect}**`);
  const allowed = new URL(page.url());
  expect(allowed.searchParams.get("code")).toBeTruthy();
  expect(allowed.searchParams.get("state")).toBe("s1");

  await page.goto(authorize);
  await page.getByRole("button", { name: "Tolak" }).click();
  await page.waitForURL(`${redirect}**`);
  expect(new URL(page.url()).searchParams.get("error")).toBe("access_denied");
});
