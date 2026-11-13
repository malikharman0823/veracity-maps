import { expect, test } from "@playwright/test"

function collectBrowserFailures(page) {
  const failures = []

  page.on("console", (message) => {
    if (message.type() === "error") failures.push(`console: ${message.text()}`)
  })
  page.on("pageerror", (error) => failures.push(`page: ${error.message}`))
  page.on("response", (response) => {
    if (response.status() >= 400 && !response.url().endsWith("/favicon.ico")) {
      failures.push(`http ${response.status()}: ${response.url()}`)
    }
  })

  return failures
}

test("public auth flow preserves input and protects private routes", async ({ page }, testInfo) => {
  const failures = collectBrowserFailures(page)

  await page.goto("/dashboard")
  await expect(page).toHaveURL(/\/login\?redirect=%2Fdashboard$/)
  await expect(page.getByRole("heading", { name: "Sign in to Veracity Maps" })).toBeVisible()

  const email = "browser.qa@example.com"
  await page.getByLabel("Email Address").fill(email)
  await page.getByRole("link", { name: "Create an account" }).click()

  await expect(page).toHaveURL(/\/register\?email=browser\.qa%40example\.com$/)
  await expect(page.getByRole("heading", { name: "Create your account" })).toBeVisible()
  await expect(page.getByLabel("Email Address")).toHaveValue(email)

  await page.getByLabel("Full Name").fill("Browser QA")
  await expect(page.getByLabel("Full Name")).toHaveValue("Browser QA")

  await page.getByRole("button", { name: /Switch to Light mode/i }).click()
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light")

  await page.getByRole("link", { name: "Sign in" }).click()
  await expect(page).toHaveURL(/\/login\?email=browser\.qa%40example\.com$/)
  await expect(page.getByLabel("Email Address")).toHaveValue(email)

  const screenshotPath = testInfo.outputPath("auth-navigation.png")
  await page.screenshot({ path: screenshotPath, fullPage: true })
  await testInfo.attach("auth-navigation", { path: screenshotPath, contentType: "image/png" })

  expect(failures, failures.join("\n")).toEqual([])
})
