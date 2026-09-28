const { chromium } = require('/opt/node22/lib/node_modules/playwright');
(async () => {
  const [base, out, ...pages] = process.argv.slice(2);
  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  for (const spec of pages) {
    const [path, file] = spec.split('=');
    await page.goto(base + path, { waitUntil: 'load' });
    await page.waitForTimeout(700);
    const title = await page.title();
    const brand = await page.locator('.nav-brand-name').first().textContent().catch(() => '');
    console.log(`${path}\ttitle="${title}"\tbrand="${brand}"`);
    await page.screenshot({ path: `${out}/${file}`, fullPage: false });
  }
  await browser.close();
})();
