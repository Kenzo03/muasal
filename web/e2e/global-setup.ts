import { execFileSync } from "node:child_process";

const compose = ["compose", "-f", "../deploy/compose.yaml", "--env-file", "../deploy/.env"];

// Waits for the stack, then creates a fresh admin through the CLI, as an installer would.
export default async function globalSetup() {
  const baseURL = process.env.E2E_BASE_URL ?? "http://localhost";
  const deadline = Date.now() + 60_000;
  for (;;) {
    try {
      if ((await fetch(`${baseURL}/api/v1/me`)).status === 401) break; // API and database answer
    } catch {
      // not up yet
    }
    if (Date.now() > deadline) throw new Error(`Muasal is not answering at ${baseURL}; run \`make up\` first`);
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  const email = `admin-${Date.now()}@example.com`;
  const out = execFileSync(
    "docker",
    [...compose, "exec", "-T", "app", "/app", "admin", "create-admin", "--email", email, "--name", "E2E Admin"],
    { encoding: "utf8" },
  );
  const link = out.match(/https?:\/\/\S+\/setup\/\S+/)?.[0];
  if (!link) throw new Error(`no setup link in: ${out}`);
  process.env.E2E_ADMIN_EMAIL = email;
  process.env.E2E_ADMIN_LINK = link;
}
