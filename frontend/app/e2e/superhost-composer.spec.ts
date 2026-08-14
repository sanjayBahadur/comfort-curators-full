import { test, expect } from "@playwright/test";

test.describe("Superhost message composer", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.sessionStorage.setItem("cc_session", "e2e-owner-session");
      window.sessionStorage.setItem("cc_role", "owner");
    });

    await page.route("**/api/v1/superhost/threads/*/stream", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: "data: [DONE]\n\n",
      });
    });
  });

  test("operator line appears after sending a message", async ({ page }) => {
    await page.route("**/api/v1/superhost/threads/*/messages", async (route) => {
      await route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({
          request_id: "req-test-01",
          status: "accepted",
          resource_id: "run-test-01",
        }),
      });
    });

    await page.route("**/api/v1/superhost/threads", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "env-test",
          version: 1,
          data: {
            thread_id: "thread-test-01",
            run_id: "run-test-01",
            created_at: new Date().toISOString(),
          },
        }),
      });
    });

    await page.goto("/dashboard");

    const globalButton = page.getByRole("button", { name: "Open Superhost" });
    if (await globalButton.isVisible({ timeout: 5000 }).catch(() => false)) {
      await globalButton.click();
    }

    const drawerMount = page.locator(".gsh-drawer-mount");
    await expect(drawerMount).toBeVisible({ timeout: 5000 });

    const input = drawerMount.locator(".superhost-mount-composer-input");
    const grant = drawerMount.getByRole("button", { name: /HAND OVER CONTROL/i });
    await grant.click();
    await input.waitFor({ state: "visible", timeout: 10000 });
    await input.fill("hello model, can you click the button?");
    await input.press("Enter");

    const task = page.locator(".superhost-task").filter({ hasText: "hello model, can you click the button?" });
    await expect(task).toBeVisible({ timeout: 5000 });
    await expect(task.locator(".superhost-task-state")).toHaveText("NOT STARTED");
  });

  test("composer is disabled when thread is not ready", async ({ page }) => {
    await page.route("**/api/v1/superhost/threads", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ code: "UNAVAILABLE", message: "test thread unavailable" }),
      });
    });

    await page.goto("/dashboard");
    await page.getByRole("button", { name: "Open Superhost" }).click();

    const input = page.locator(".gsh-drawer-mount .superhost-mount-composer-input");
    await expect(input).toBeVisible();
    await expect(input).toBeDisabled();
  });
});
