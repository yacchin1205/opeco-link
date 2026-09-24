import { test, expect, screenshot } from "./fixtures.js";

test("empty state, light branding, and device management copy", async ({ page }) => {
  await page.goto("/");
  const management = page.getByRole("button", { name: "デバイスグループの管理（1台）", exact: true });
  await expect(management).toBeEnabled();
  await expect(page.getByRole("heading", { name: "セッションがありません" })).toBeVisible();
  await expect(page.locator("#device-management")).toBeHidden();
  await expect(page.locator("#device-count")).toBeHidden();
  await expect(page.locator(".brand")).toHaveText("opeco.link");
  await expect(page.locator(".brand img")).toHaveAttribute("src", "/opeco/outline.svg");
  await expect(page.locator(".brand img")).toHaveCSS("opacity", "0.6");
  await expect(page.locator(".empty-mark")).toHaveCSS("background-color", "rgb(136, 136, 136)");
  await expect(page.locator("body")).toHaveCSS("background-color", "rgb(242, 242, 247)");
  await screenshot(page, "empty-dark-preference");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("body")).toHaveCSS("background-color", "rgb(242, 242, 247)");

  await management.click();
  await expect(page.getByText("この端末を、すでに使っている端末と同じグループに追加できます。", { exact: true })).toBeVisible();
  const add = page.getByRole("button", { name: "このデバイスを別のグループに追加", exact: true });
  await expect(add).toBeVisible();
  await expect(page.locator("#device-management")).not.toContainText("読み取って");
  await screenshot(page, "device-management");
  await page.getByRole("button", { name: "閉じる", exact: true }).click();
  await expect(page.locator("#device-management")).toBeHidden();
  await expect(page.getByRole("heading", { name: "セッションがありません" })).toBeVisible();
  await management.click();
  await add.click();
  await expect(page.getByRole("heading", { name: "別のグループへの追加待ち" })).toBeVisible();
  await expect(page.getByRole("img", { name: "このデバイスをグループに追加するQRコード" })).toBeVisible();
  await expect(page.getByText("追加先のグループに所属しているデバイスでこのQRコードを読み取ってください。", { exact: true })).toBeVisible();
  await screenshot(page, "device-request-qr");
});

test("session notification, answer, feedback, and reload round trip", async ({ page, createSession }) => {
  const session = await createSession("Web E2E session");
  await page.goto(session.pairingURL);
  await expect(page.getByText("セッションへ参加しました。", { exact: true })).toBeVisible();
  await session.call("session_wait_for_device", { timeout_seconds: 5 });
  await session.call("status", { status: "Checking the browser flow" });
  const card = page.getByRole("article").filter({ has: page.getByRole("heading", { name: "Web E2E session", exact: true }) });
  await expect(card.getByText("Checking the browser flow", { exact: true })).toBeVisible();
  await expect(card.getByText("SESSION", { exact: true })).toHaveCount(0);
  await expect(card.locator(".session-opeco")).toHaveAttribute("src", "/opeco/blue.svg");
  await expect(card.locator(".unresolved-count")).toBeHidden();
  const notification = await session.call("notify", { message: "Notification from the agent" });
  await expect(card.getByText("Notification from the agent", { exact: true })).toBeVisible();
  await expect(card.locator(".unresolved-count")).toHaveText("1");
  await screenshot(page, "session-notification");
  await card.getByRole("button", { name: "通知を消す", exact: true }).click();
  await expect(card.getByText("Notification from the agent", { exact: true })).toHaveCount(0);
  expect(await session.call("responses_wait", { timeout_seconds: 5 })).toMatchObject({
    responses: [expect.objectContaining({ type: "dismiss", itemId: notification.item_id })],
  });

  const question = await session.call("request", { prompt: "Continue the browser test?", options: ["続行", "中止"] });
  await expect(card.getByText("Continue the browser test?", { exact: true })).toBeVisible();
  await screenshot(page, "session-question");
  await card.getByRole("button", { name: "続行", exact: true }).click();
  await expect(card.getByText("Continue the browser test?", { exact: true })).toHaveCount(0);
  expect(await session.call("responses_wait", { timeout_seconds: 5 })).toMatchObject({
    responses: [expect.objectContaining({
      type: "response", requestId: question.request_id, optionId: (question.choices as { id: string }[])[0].id,
    })],
  });

  await card.getByRole("button", { name: "メッセージを送る", exact: true }).click();
  await card.getByRole("textbox", { name: "このセッションへメッセージを送る" }).fill("Message from the browser");
  await screenshot(page, "feedback-compose");
  await card.getByRole("button", { name: "送信", exact: true }).click();
  await expect(card.getByRole("textbox")).toBeHidden();
  expect(await session.call("responses_wait", { timeout_seconds: 5 })).toMatchObject({
    responses: [expect.objectContaining({ type: "feedback", message: "Message from the browser" })],
  });
  await expect(card.locator(".unresolved-count")).toBeHidden();
  await page.reload();
  await expect(card.getByText("応答を送信しました", { exact: true })).toBeVisible();
  await expect(card.locator(".unresolved-count")).toBeHidden();
  await expect(page.locator("#device-management")).toBeHidden();
  await screenshot(page, "session-reloaded");
});

test("device addition inherits sessions and removal clears the badge", async ({ page, browser, baseURL, createSession }) => {
  const session = await createSession("Shared session");
  await page.goto(session.pairingURL);
  await expect(page.getByText("セッションへ参加しました。", { exact: true })).toBeVisible();
  await session.call("session_wait_for_device", { timeout_seconds: 5 });
  await session.call("status", { status: "Shared status" });
  const secondContext = await browser.newContext({
    baseURL, viewport: page.viewportSize()!, colorScheme: "dark", permissions: ["clipboard-read", "clipboard-write"],
  });
  try {
    const second = await secondContext.newPage();
    await second.goto("/");
    await second.getByRole("button", { name: "デバイスグループの管理（1台）", exact: true }).click();
    await second.getByRole("button", { name: "このデバイスを別のグループに追加", exact: true }).click();
    await expect(second.getByRole("img", { name: "このデバイスをグループに追加するQRコード" })).toBeVisible();
    await second.getByRole("button", { name: "グループ追加用リンクを共有", exact: true }).click();
    const link = await second.evaluate(() => navigator.clipboard.readText());
    expect(new URL(link).origin).toBe(baseURL);
    expect(new URL(link).pathname).toBe("/device");
    await page.goto(link);
    await expect(page.getByText("デバイスをグループへ追加しました。", { exact: true })).toBeVisible();
    await expect(second.getByText("デバイスグループへ追加されました。", { exact: true })).toBeVisible();
    await expect(second.getByRole("article")).toHaveCount(1);
    await session.call("status", { status: "Shared status after device addition" });
    await expect(second.getByRole("heading", { name: "Shared session", exact: true })).toBeVisible();
    await expect(second.getByText("Shared status after device addition", { exact: true })).toBeVisible();
    await expect(page.getByText("Shared status after device addition", { exact: true })).toBeVisible();
    await expect(second.locator("#join-device")).toBeHidden();
    await expect(page.locator("#device-count")).toHaveText("2");
    await expect(page.locator("#device-count")).toHaveCSS("background-color", "rgb(107, 107, 115)");
    await page.getByRole("button", { name: "デバイスグループの管理（2台）", exact: true }).click();
    await expect(page.locator(".device-row")).toHaveCount(2);
    const add = page.getByRole("button", { name: "このデバイスを別のグループに追加", exact: true });
    const leave = page.getByRole("button", { name: "このデバイスをグループから除外", exact: true });
    await expect(leave).toBeVisible();
    const addBounds = (await add.boundingBox())!;
    const leaveBounds = (await leave.boundingBox())!;
    expect(leaveBounds.y).toBeGreaterThan(addBounds.y + addBounds.height);
    expect(leaveBounds.x).toBe(addBounds.x);
    await screenshot(page, "two-devices");
    await screenshot(second, "inherited-session");
    const remove = page.getByRole("button", { name: /^デバイス .* をグループから除外$/ });
    // A press during the periodic synchronization waits for it instead of being rejected.
    const dialogPromise = page.waitForEvent("dialog");
    const click = remove.click();
    const dialog = await dialogPromise;
    expect(dialog.message()).toBe("このデバイスをグループから除外しますか？");
    await dialog.accept();
    await click;
    await expect(page.locator(".device-row")).toHaveCount(1);
    await expect(page.locator("#device-count")).toBeHidden();
    await expect(second.getByRole("heading", { name: "セッションがありません", exact: true })).toBeVisible();
    await screenshot(page, "device-removed");
    await page.getByRole("button", { name: "閉じる", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Shared session", exact: true })).toBeVisible();
  } finally {
    await secondContext.close();
  }
});

test("session grid and preset icons follow the viewport and session hue", async ({ page, createSession }, testInfo) => {
  for (const [title, color] of [["Blue session", "#d0dbf2"], ["Green session", "#d0f2d0"], ["Red session", "#f2d0d0"]]) {
    const session = await createSession(title, color);
    await page.goto(session.pairingURL);
    await expect(page.getByText("セッションへ参加しました。", { exact: true })).toBeVisible();
    await session.call("session_wait_for_device", { timeout_seconds: 5 });
    await session.call("status", { status: `${title} is working` });
    await expect(page.getByRole("heading", { name: title, exact: true })).toBeVisible();
  }
  await expect(page.getByRole("article")).toHaveCount(3);
  for (const palette of ["blue", "green", "red"]) {
    const icon = page.locator(`.session-opeco[src="/opeco/${palette}.svg"]`);
    await expect(icon).toBeVisible();
    await expect.poll(() => icon.evaluate((element: HTMLImageElement) => element.complete && element.naturalWidth > 0)).toBe(true);
  }
  const columns = { phone: 1, "tablet-portrait": 2, "tablet-landscape": 3 }[testInfo.project.name];
  expect(await page.locator("#cards").evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(" ").length)).toBe(columns);
  const layout = await page.evaluate(() => {
    const logo = document.querySelector(".brand")!.getBoundingClientRect();
    const actions = document.querySelector(".header-actions")!.getBoundingClientRect();
    return { overflow: document.documentElement.scrollWidth > innerWidth, headerOverlaps: logo.right > actions.left };
  });
  expect(layout).toEqual({ overflow: false, headerOverlaps: false });
  await screenshot(page, "session-grid");
});
