import { execFileSync } from "node:child_process";

const compose = ["compose", "-f", "../deploy/compose.yaml", "--env-file", "../deploy/.env"];

// createAdmin makes an admin through the CLI, as an installer would, and returns their setup link.
function createAdmin(name: string) {
  const email = `${name.toLowerCase().replaceAll(" ", "-")}-${Date.now()}@example.com`;
  const out = execFileSync(
    "docker",
    [...compose, "exec", "-T", "app", "/app", "admin", "create-admin", "--email", email, "--name", name],
    { encoding: "utf8" },
  );
  const link = out.match(/https?:\/\/\S+\/setup\/\S+/)?.[0];
  if (!link) throw new Error(`no setup link in: ${out}`);
  return { email, link };
}

// Waits for the stack, then creates one fresh admin per test file.
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
  const signin = createAdmin("E2E Admin");
  process.env.E2E_ADMIN_EMAIL = signin.email;
  process.env.E2E_ADMIN_LINK = signin.link;
  const tree = createAdmin("Tree Admin");
  process.env.E2E_TREE_ADMIN_EMAIL = tree.email;
  process.env.E2E_TREE_ADMIN_LINK = tree.link;
  const web = createAdmin("Web Admin");
  process.env.E2E_WEB_ADMIN_EMAIL = web.email;
  process.env.E2E_WEB_ADMIN_LINK = web.link;
  const tickets = createAdmin("Ticket Admin");
  process.env.E2E_TICKET_ADMIN_EMAIL = tickets.email;
  process.env.E2E_TICKET_ADMIN_LINK = tickets.link;
  const decisions = createAdmin("Decision Admin");
  process.env.E2E_DECISION_ADMIN_EMAIL = decisions.email;
  process.env.E2E_DECISION_ADMIN_LINK = decisions.link;
  const aiAdmin = createAdmin("AI Admin");
  process.env.E2E_AI_ADMIN_EMAIL = aiAdmin.email;
  process.env.E2E_AI_ADMIN_LINK = aiAdmin.link;
}
