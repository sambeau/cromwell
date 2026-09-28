// The SPEC-019 walkthrough (DoD 3). Run setup.sh and chat.sh first, then:
//   node docs/walkthrough-spec-019/walk.js docs/walkthrough-spec-019
// No AI provider is used. The browser plays the person; the chat agent's
// relay is a plain MCP JSON-RPC call. Screenshots go in the directory given.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');
const fs = require('fs');

const base = 'http://127.0.0.1:8819';
const out = process.argv[2] || '.';
const log = [];

const mcp = (name, args) => {
  const body = JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } });
  const raw = execSync(`curl -s --noproxy '*' -H 'Content-Type: application/json' -d @- ${base}/mcp`, { input: body, encoding: 'utf8' });
  const res = JSON.parse(raw).result;
  if (res.isError) throw new Error(`${name}: ${res.content[0].text}`);
  log.push(`» ${name} ${JSON.stringify(args)}\n  ${JSON.stringify(res.structuredContent)}`);
  return res.structuredContent;
};

(async () => {
  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const shot = async (file, locator) => {
    await page.waitForTimeout(500);
    if (locator) await page.locator(locator).first().scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${out}/${file}`, fullPage: false });
    console.log(`${file}\t${page.url().replace(base, '')}\t"${await page.title()}"`);
  };
  // Pages are boosted by HTMX, so a form's answer is swapped in after the
  // load event: wait for the text rather than the navigation.
  const expectText = async (text) => {
    try {
      await page.waitForFunction((t) => document.body.innerText.includes(t) || document.documentElement.innerHTML.includes(t), text, { timeout: 8000 });
    } catch (e) {
      await page.screenshot({ path: `${out}/failed.png` });
      throw new Error(`expected "${text}" on ${page.url()}`);
    }
  };

  // 1. A person reports a bug from the feature's page.
  await page.goto(base + '/ui/id/FEAT-001');
  await page.getByLabel('More actions').click();
  await page.getByRole('button', { name: 'Report a bug…' }).click();
  await page.fill('#bug-title', 'The greeting ignores the time zone');
  await page.fill('#bug-steps', 'Set the browser to Europe/London\nOpen the home page at 08:00 London time');
  await page.fill('#bug-expected', 'It says good morning.');
  await page.fill('#bug-actual', 'It says good evening, the greeting for the server’s time zone.');
  await shot('01-report-dialog.png');
  await page.getByRole('button', { name: 'Report the bug' }).click();
  await page.waitForLoadState('load');
  await expectText('BUG-002 was reported. It waits in the triage queue');
  await expectText('Waiting for triage');
  await shot('02-bug-reported.png');

  // 2. The triage queue, with its count in the navigation.
  await page.goto(base + '/ui/triage');
  await expectText('The evening greeting says good morning');
  await expectText('Reported by the chat agent.');
  await expectText('Reported by sam in the web UI.');
  await page.waitForFunction(() => document.querySelector('#triage-badge').textContent.trim() === '2');
  await shot('03-triage-queue.png');

  // 3. The Inbox says so too.
  await page.goto(base + '/ui/inbox');
  await page.waitForSelector('text=2 bug reports are waiting for triage.');
  await shot('04-inbox-count.png');

  // 4. The person accepts BUG-002 in the queue.
  await page.goto(base + '/ui/triage');
  await page.locator('#bug-BUG-002').getByRole('button', { name: /Accept it/ }).click();
  await page.waitForLoadState('load');
  await expectText('BUG-002 was accepted.');
  await shot('05-accepted.png');

  // 5. The chat agent relays the person's rejection of BUG-001, quoting them.
  mcp('relay_triage', { bug: 'BUG-001', decision: 'reject', reason: 'It is the same fault as the time-zone bug, seen from the other side.',
    quote: 'Reject the evening one, it is just the time-zone bug again.' });
  mcp('get_bug', { bug: 'BUG-001' });
  await page.goto(base + '/ui/triage');
  await expectText('Nothing is waiting for triage');
  await expectText('Their words: “Reject the evening one, it is just the time-zone bug again.”');
  await shot('06-relayed-rejection.png', '#recently-triaged');
  await page.goto(base + '/ui/id/BUG-001');
  await expectText('a person, relayed by the chat agent');
  await shot('07-rejected-bug.png');

  // 6. The accepted bug can be sent. The send screen says its report is the spec.
  await page.goto(base + '/ui/id/BUG-002');
  await expectText('Accepted');
  await shot('08-accepted-bug.png');
  await page.getByRole('link', { name: 'Send to development' }).first().click();
  await page.waitForLoadState('load');
  await expectText('The bug report is the specification, so this step is skipped.');
  await expectText('Review the bug report as a specification');
  await shot('09-send-screen.png');
  await page.getByRole('button', { name: 'Send to development' }).click();
  await page.waitForLoadState('load');
  await expectText('Sent to development');
  await shot('10-sent.png');

  // 7. Sending submitted the report, and its review waits in the queue.
  await page.goto(base + '/ui/id/BUG-002-bug-report');
  await expectText('reviewing');
  await shot('11-report-in-review.png');

  fs.writeFileSync(`${out}/relay-output.txt`, log.join('\n') + '\n');
  await browser.close();
})().catch((e) => { console.error(e); process.exit(1); });
