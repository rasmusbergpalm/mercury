// Watching a game: open the lobby, press Watch on the first game, and go
// through every tab and every chart on it, as a spectator does. Fails on
// any error the page throws, or if the game doesn't stay open.
//
//   node web/e2e/watch.js http://127.0.0.1:8080
//
// Run by cmd/server's TestWatchGame. Exits 3 when Playwright isn't there.
let chromium;
try {
  ({ chromium } = require('playwright'));
} catch (e) {
  console.log('playwright not installed');
  process.exit(3);
}

(async () => {
  const base = process.argv[2];
  const errors = [];
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } });
  page.on('pageerror', (e) => errors.push(`page error: ${e.message}`));
  page.on('console', (m) => {
    // a resource the sandbox can't fetch (a web font) is not the page's fault
    if (m.type() === 'error' && !m.text().startsWith('Failed to load resource')) errors.push(`console: ${m.text()}`);
  });
  const fail = async (why) => {
    console.log(why);
    errors.forEach((e) => console.log(e));
    await browser.close();
    process.exit(1);
  };
  const settle = () => page.waitForTimeout(400);

  await page.goto(base + '/');
  await page.waitForSelector('button[data-open]', { timeout: 15000 });
  await page.click('button[data-open]');
  // the game is open once its tabs show, or the page has thrown and gone back
  // to the lobby; on a busy machine that can take a while
  await page.waitForFunction(() => {
    const tab = document.querySelector('button[data-tab]');
    return (/^#\/game\//.test(location.hash) && tab && tab.offsetParent !== null) || location.hash === '#/' && (document.getElementById('lobby-err') || {}).textContent;
  }, null, { timeout: 20000 }).catch(() => {});
  await page.waitForTimeout(1500); // then each nation's history
  const hash = await page.evaluate(() => location.hash);
  if (!/^#\/game\/[0-9a-f]+$/.test(hash)) {
    const shown = await page.evaluate(() => (document.getElementById('lobby-err') || {}).textContent || '');
    await fail(`the game didn't open: at ${hash}; the lobby says "${shown.trim()}"`);
  }
  // the tabs a spectator sees (some are only for a nation's player)
  const visible = (sel) => page.$$eval(sel, (bs) => bs.filter((b) => b.offsetParent !== null).map((b) => [b.dataset.tab || b.dataset.pick, b.dataset.val || '']));
  const tabs = await visible('button[data-tab]');
  if (tabs.length < 2) await fail(`only ${tabs.length} tab to watch`);
  for (const [tab] of tabs) {
    await page.click(`button[data-tab="${tab}"]`);
    await settle();
    // every chart the tab offers (People, GDP, ..., Might)
    for (const [key, val] of await visible('button[data-pick]')) {
      // a locator, found again at the click: the page redraws its buttons
      // as the game runs, and a handle to the old one is left detached
      const b = page.locator(`button[data-pick="${key}"][data-val="${val}"]`).first();
      if (await b.isVisible()) {
        await b.click({ timeout: 5000 });
        await settle();
      }
    }
  }
  if (errors.length) await fail('the page threw while watching');
  console.log('ok');
  await browser.close();
})().catch(async (e) => {
  console.log(`script error: ${e.message}`);
  process.exit(1);
});
