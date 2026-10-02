// The SPEC-021 stage 2 walkthrough. Run setup.sh first, then:
//   node docs/walkthrough-spec-021-stage2/walk.js docs/walkthrough-spec-021-stage2
// No AI provider is used. The browser plays the person; the chat agent is a
// plain MCP JSON-RPC call. Screenshots go in the directory given, and the MCP
// log beside them.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');
const fs = require('fs');

const base = 'http://127.0.0.1:8830';
const PG = process.env.PG || 'postgres://postgres@localhost:54329';
const out = process.argv[2] || '.';
const log = [];

const mcpRaw = (method, params) => {
  const body = JSON.stringify({ jsonrpc: '2.0', id: 1, method, params });
  return JSON.parse(execSync(`curl -s --noproxy '*' -H 'Content-Type: application/json' -d @- ${base}/mcp`, { input: body, encoding: 'utf8' }));
};
const mcp = (name, args) => {
  const reply = mcpRaw('tools/call', { name, arguments: args });
  if (reply.error) {
    log.push(`» ${name} ${JSON.stringify(args)}\n  JSON-RPC error ${reply.error.code}: ${reply.error.message}\n`);
    return { error: reply.error.message };
  }
  const res = reply.result;
  log.push(`» ${name} ${JSON.stringify(args)}\n  ${res.isError ? 'refused: ' + res.content[0].text : JSON.stringify(res.structuredContent, null, 2)}\n`);
  if (res.isError) return { error: res.content[0].text };
  return res.structuredContent;
};
const sql = (q) => execSync(`psql "${PG}/m14s2_demo" -qAt -c "${q}"`, { encoding: 'utf8' }).trim();

const FINDINGS = `## Answer

Yes. The import screen can show a progress bar that updates every second: the server reports progress in 100-row steps, and a ten-thousand-row import takes about fourteen seconds, so the bar moves about every second.

## What we found

- The import reports its progress after every 100 rows.
- A poll once a second is enough to keep the bar moving smoothly.

## How we found out

A throwaway script in the working copy ran a ten-thousand-row import and logged the progress calls with their times.

## What to do next

Write the design for the import screen on the assumption that polling once a second is enough.
`;

(async () => {
  // 1. The chat agent plans the work and writes a question down.
  mcp('create_initiative', { slug: 'imports', name: 'Imports', description: "Bring a customer's existing records into the product." });
  mcp('create_feature', { initiative_path: 'imports', slug: 'bulk-import', name: 'Bulk import', description: 'Import a whole customer list in one go.', design_document: false });
  mcp('create_spike', { on: 'imports/bulk-import', question: 'Can the import screen show a progress bar that updates every second?' });

  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const shot = async (file, locator) => {
    await page.waitForTimeout(400);
    if (locator) {
      // A viewport shot, scrolled to the element.
      await page.locator(locator).first().scrollIntoViewIfNeeded();
      await page.screenshot({ path: `${out}/${file}`, fullPage: false });
    } else {
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({ path: `${out}/${file}`, fullPage: true });
    }
    console.log(`${file}\t${page.url().replace(base, '')}`);
  };
  const expectText = async (text, timeout = 15000) => {
    try {
      await page.waitForFunction((t) => document.body.innerText.includes(t), text, { timeout });
    } catch (e) {
      await page.screenshot({ path: `${out}/failed.png`, fullPage: true });
      throw new Error(`expected "${text}" on ${page.url()}`);
    }
  };
  const reloadUntil = async (text, tries = 60) => {
    for (let i = 0; i < tries; i++) {
      await page.reload();
      if ((await page.locator('body').innerText()).includes(text)) return;
      await page.waitForTimeout(1000);
    }
    await page.screenshot({ path: `${out}/failed.png`, fullPage: true });
    throw new Error(`never saw "${text}" on ${page.url()}`);
  };
  const startScreen = async (id) => {
    await page.goto(`${base}/ui/s/${id}`);
    await page.locator('a', { hasText: 'Start this spike' }).first().click();
    await expectText('Who runs it');
  };

  // A person writes a second spike down on the feature's page.
  await page.goto(`${base}/ui/f/imports/bulk-import`);
  await expectText('Spikes');
  await page.locator('[data-open-dialog="dlg-new-spike"]').first().click();
  await page.fill('#spike-question', 'Does the list endpoint return a 10,000-item customer list in under two seconds?');
  await shot('01-new-spike-dialog.png');
  await page.locator('#dlg-new-spike button[type=submit]').click();
  await expectText('SPK-002');
  await shot('02-spikes-on-the-feature.png', 'text=SPK-002');

  // The chat agent writes two more, for the time box and the stale claim.
  mcp('create_spike', { on: 'imports/bulk-import', question: 'Can the importer resume after a dropped connection?' });
  mcp('create_spike', { on: 'imports/bulk-import', question: 'Can an import be cancelled half way without leaving rows behind?' });

  // 2. The start screen for spike 1, for the chat agent.
  await startScreen('SPK-001');
  await page.check('#start-ex-chat');
  await expectText('Work done in chat or by hand');
  await shot('03-start-screen-chat.png');
  await page.locator('form[action="/ui/spikes/start"] button[type=submit]').click();
  await expectText('has started, with a time box of 4 hours');
  await shot('04-started-in-chat.png');

  // 3. The chat agent tries what it can't do, then claims, saves, and we look.
  mcp('start_spike', { spike: 'SPK-001' });
  mcp('close_spike', { spike: 'SPK-001' });
  mcp('claim_task', { task: 'SPK-001' });
  const claim = mcp('claim_spike', { spike: 'SPK-001' });
  if (claim.error) throw new Error('claim_spike refused: ' + claim.error);
  log.push(`# claim_spike returned: working_copy ${JSON.stringify(claim.working_copy)}; contract keys ${Object.keys(claim.contract || {}).join(', ')}; deadline ${claim.deadline}; time_left "${claim.time_left}"; ${(claim.rules || []).length} rules\n`);
  mcp('save_spike_findings', { spike: 'SPK-001', findings: '## Answer\n\nNot yet: the server reports progress, but how often has not been measured.\n\n## What we found\n\n- The import reports its progress after every 100 rows.\n' });
  await page.goto(`${base}/ui/s/SPK-001`);
  await expectText('Release the claim');
  await shot('05-chat-spike-claimed.png');

  // 4. Submit: first with an incomplete write-up, which is refused.
  mcp('submit_spike', { spike: 'SPK-001', findings: '## Answer\n\nYes.\n' });
  mcp('submit_spike', { spike: 'SPK-001', findings: FINDINGS });
  await page.goto(`${base}/ui/s/SPK-001`);
  await expectText("This spike ran in chat, so its tokens weren't measured.");
  await shot('06-chat-spike-ended.png');
  await page.locator('a[href^="/ui/d/"]').first().click();
  await expectText('How this spike ended');
  await shot('07-chat-findings.png');
  await page.goto(`${base}/ui/s/SPK-001`);
  await page.locator('button', { hasText: 'The question is answered' }).click();
  await expectText('the question is answered');
  await shot('08-chat-spike-closed.png');

  // 5. Spike 2, started for a person, with a time box of 2 hours.
  await startScreen('SPK-002');
  await page.check('#start-ex-person');
  await page.fill('#start-time-box', '2');
  await shot('09-start-screen-person.png');
  await page.locator('form[action="/ui/spikes/start"] button[type=submit]').click();
  await expectText('has started, with a time box of 2 hours');
  await shot('10-person-spike-started.png');
  await page.locator('button', { hasText: "I'll run this spike" }).click();
  await expectText("You are running SPK-002");
  await shot('11-person-claimed-before-saving.png');
  await page.fill('#spike-findings-text', FINDINGS);
  await page.locator('button', { hasText: 'Save findings' }).click();
  await expectText('You are running SPK-002');
  await shot('12-person-saved-draft.png', '#spike-findings-text');
  await page.locator('button', { hasText: "I've finished" }).click();
  await expectText('Ended, waiting to be read');
  await shot('13-person-spike-ended.png');
  await page.locator('button', { hasText: 'The question is answered' }).click();
  await expectText('Closed by sam, who also ran it.');
  await shot('14-person-spike-closed.png');

  // 6. Spike 3, claimed by chat, then its time box runs out.
  await startScreen('SPK-003');
  await page.check('#start-ex-chat');
  await page.locator('form[action="/ui/spikes/start"] button[type=submit]').click();
  await expectText('has started, with a time box of 4 hours');
  mcp('claim_spike', { spike: 'SPK-003' });
  mcp('save_spike_findings', { spike: 'SPK-003', findings: '## Answer\n\nNot yet: only the first half of the question has been looked at.\n\n## What we found\n\n- A dropped connection leaves the import at the last 100-row step.\n' });
  await page.goto(`${base}/ui/s/SPK-003`);
  await shot('15-time-box-before.png');
  sql("UPDATE spikes SET deadline_at = now() - interval '1 minute' WHERE public_id = 'SPK-003'; UPDATE work_claims SET deadline_at = now() - interval '1 minute' WHERE ref_id = (SELECT id FROM spikes WHERE public_id = 'SPK-003')");
  log.push('# SPK-003: spikes.deadline_at and work_claims.deadline_at backdated by SQL; waiting for the heartbeat\n');
  await reloadUntil('Ended, waiting to be read');
  await expectText('This spike has ended: it reached its time box');
  await shot('16-time-box-ended.png');
  await page.locator('a[href^="/ui/d/"]').first().click();
  await expectText('How this spike ended');
  await shot('17-time-box-findings.png');
  mcp('get_spike', { spike: 'SPK-003' });
  mcp('claim_spike', { spike: 'SPK-003' });

  // 7. Spike 4: claimed in chat, then quiet for more than 24 hours.
  await startScreen('SPK-004');
  await page.check('#start-ex-chat');
  await page.locator('form[action="/ui/spikes/start"] button[type=submit]').click();
  await expectText('has started, with a time box of 4 hours');
  mcp('claim_spike', { spike: 'SPK-004' });
  sql("UPDATE work_claims SET last_activity_at = now() - interval '26 hours', claimed_at = now() - interval '26 hours' WHERE ref_id = (SELECT id FROM spikes WHERE public_id = 'SPK-004')");
  log.push('# SPK-004: work_claims.last_activity_at backdated 26 hours by SQL; waiting for the heartbeat\n');
  let stale = false;
  for (let i = 0; i < 30 && !stale; i++) {
    await page.goto(`${base}/ui/inbox`);
    stale = (await page.locator('body').innerText()).includes('Release the claim');
    if (!stale) await page.waitForTimeout(1000);
  }
  await shot('18-inbox-claim-stale.png');
  await page.goto(`${base}/ui/s/SPK-004`);
  await shot('19-stale-spike-page.png');

  // The list, with the executor words.
  await page.goto(`${base}/ui/spikes`);
  await shot('20-spikes-list.png');

  mcp('get_spike', { spike: 'SPK-004' });
  fs.writeFileSync(`${out}/mcp-output.txt`, log.join('\n') + '\n');
  await browser.close();
})().catch((e) => { console.error(e); fs.writeFileSync(`${out}/mcp-output.txt`, log.join('\n') + '\n'); process.exit(1); });
