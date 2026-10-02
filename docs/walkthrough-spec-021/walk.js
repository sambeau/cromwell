// The SPEC-021 walkthrough (DoD 8). Run setup.sh first, then:
//   node docs/walkthrough-spec-021/walk.js docs/walkthrough-spec-021
// No AI provider is used: fake_provider.py plays the spike's agent. The
// browser plays the person; the chat agent is a plain MCP JSON-RPC call.
// Screenshots go in the directory given, and the MCP log beside them.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');
const fs = require('fs');

const base = 'http://127.0.0.1:8820';
const out = process.argv[2] || '.';
const log = [];

const mcp = (name, args) => {
  const body = JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } });
  const raw = execSync(`curl -s --noproxy '*' -H 'Content-Type: application/json' -d @- ${base}/mcp`, { input: body, encoding: 'utf8' });
  const reply = JSON.parse(raw);
  if (reply.error) {
    log.push(`» ${name} ${JSON.stringify(args)}\n  JSON-RPC error ${reply.error.code}: ${reply.error.message}`);
    return { error: reply.error.message };
  }
  const res = reply.result;
  log.push(`» ${name} ${JSON.stringify(args)}\n  ${res.isError ? 'refused: ' + res.content[0].text : JSON.stringify(res.structuredContent)}`);
  if (res.isError) return { error: res.content[0].text };
  return res.structuredContent;
};

(async () => {
  // The chat agent plans the work and writes a question down. It can't
  // start the spike: there is no tool for that.
  mcp('create_initiative', { slug: 'imports', name: 'Imports', description: 'Bring a customer\'s existing records into the product.' });
  mcp('create_feature', { initiative_path: 'imports', slug: 'bulk-import', name: 'Bulk import', description: 'Import a whole customer list in one go.', design_document: false });
  const chat = mcp('create_spike', { on: 'imports/bulk-import', question: 'Can the import screen show a progress bar that updates every second?' });
  mcp('start_spike', { spike: chat.id || 'SPK-001' });

  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const shot = async (file, locator) => {
    await page.waitForTimeout(400);
    if (locator) await page.locator(locator).first().scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${out}/${file}`, fullPage: false });
    console.log(`${file}\t${page.url().replace(base, '')}`);
  };
  const expectText = async (text, timeout = 15000) => {
    try {
      await page.waitForFunction((t) => document.body.innerText.includes(t), text, { timeout });
    } catch (e) {
      await page.screenshot({ path: `${out}/failed.png` });
      throw new Error(`expected "${text}" on ${page.url()}`);
    }
  };
  const reloadUntil = async (text) => {
    for (let i = 0; i < 60; i++) {
      await page.reload();
      if ((await page.locator('body').innerText()).includes(text)) return;
      await page.waitForTimeout(1000);
    }
    await page.screenshot({ path: `${out}/failed.png` });
    throw new Error(`never saw "${text}" on ${page.url()}`);
  };

  // 1. A person writes a spike down on the feature's page.
  await page.goto(`${base}/ui/f/imports/bulk-import`);
  await expectText('Spikes');
  await page.locator('[data-open-dialog="dlg-new-spike"]').first().click();
  await page.fill('#spike-question', 'Is the list endpoint fast enough to fetch a 10,000-item customer list whole, in under two seconds?');
  await page.fill('#spike-budget', '20000');
  await shot('01-new-spike-dialog.png');
  await page.locator('#dlg-new-spike button[type=submit]').click();
  await expectText('SPK-002');
  await shot('02-spikes-on-the-feature.png', 'text=SPK-002');

  // 2. Its page, before it starts.
  await page.goto(`${base}/ui/s/SPK-002`);
  await expectText("This spike hasn't started yet.");
  await shot('03-spike-not-started.png');

  // 3. The start screen: what will run, and the budget.
  await page.locator('a', { hasText: 'Start this spike' }).first().click();
  await expectText('Who will run it');
  await shot('04-start-screen.png');
  await page.locator('#start-budget').scrollIntoViewIfNeeded();
  await shot('05-start-screen-budget.png', 'text=Where it works');

  // 4. Start it. The scripted agent saves a draft, then keeps working and
  // never finishes, so the budget stops it.
  await page.locator('form[action="/ui/spikes/start"] button[type=submit]').click();
  await expectText('has started, with a budget of 20,000 tokens');
  await shot('06-started.png');
  await reloadUntil('It\'s waiting for you to read the findings.');
  await shot('07-stopped-at-its-budget.png');
  await page.locator('text=Tokens').first().scrollIntoViewIfNeeded();
  await shot('08-tokens-and-findings.png', 'text=Findings');

  // 5. The findings, written up at the stop.
  await page.locator('a[href^="/ui/d/"]').first().click();
  await expectText('How this spike ended');
  await page.locator('text=How this spike ended').first().scrollIntoViewIfNeeded();
  await shot('09-findings-at-the-stop.png', 'text=How this spike ended');

  // 6. The run's transcript ends with the stop.
  await page.goto(`${base}/ui/s/SPK-002`);
  await page.locator('a[href^="/ui/run/"]').first().click();
  await expectText('stopped here because it reached its budget');
  await page.locator('text=stopped here because').first().scrollIntoViewIfNeeded();
  await shot('10-run-stopped.png', 'text=stopped here because');

  // 7. The person decides the question deserves a second, separately
  // budgeted spike.
  await page.goto(`${base}/ui/s/SPK-002`);
  await page.fill('#again-budget', '60000');
  await shot('11-ask-again.png', '#again-budget');
  await page.locator('form[action="/ui/spikes/close"]:has(input[value=again]) button').click();
  await expectText('Start SPK-003');
  await shot('12-second-spike-start-screen.png', 'text=What the agent will be given');
  await page.locator('form[action="/ui/spikes/start"] button[type=submit]').click();
  await expectText('has started, with a budget of 60,000 tokens');
  await reloadUntil('it reached a conclusion');
  await shot('13-second-spike-concluded.png');
  await page.locator('a[href^="/ui/d/"]').first().click();
  await expectText('well inside the two-second target');
  await shot('14-findings-concluded.png');

  // 8. The person reads the findings and closes the spike as answered.
  await page.goto(`${base}/ui/s/SPK-003`);
  await page.locator('button', { hasText: 'The question is answered' }).click();
  await expectText('the question is answered');
  await shot('15-closed-answered.png');

  // 9. Where spikes show: the feature's page, its timeline, and the list.
  await page.goto(`${base}/ui/f/imports/bulk-import`);
  await expectText('Spike SPK-003');
  await shot('16-feature-timeline.png', 'text=Spike SPK-002');
  await page.goto(`${base}/ui/spikes`);
  await shot('17-spikes-list.png');

  // 10. The chat agent reads what happened.
  mcp('get_spike', { spike: 'SPK-003' });

  fs.writeFileSync(`${out}/mcp-output.txt`, log.join('\n') + '\n');
  await browser.close();
})().catch((e) => { console.error(e); fs.writeFileSync(`${out}/mcp-output.txt`, log.join('\n') + '\n'); process.exit(1); });
