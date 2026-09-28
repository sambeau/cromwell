// The SPEC-017 walkthrough (DoD 3). Run setup.sh and chat.sh first, then:
//   node docs/walkthrough-spec-017/walk.js docs/walkthrough-spec-017
// No AI provider is used. The browser plays the person; chat2.sh plays the
// chat agent, over plain MCP JSON-RPC. Screenshots go in the directory given.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');

const base = 'http://127.0.0.1:8817';
const out = process.argv[2] || '.';
const here = __dirname;

const mcp = (name, args) => {
  const body = JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } });
  const raw = execSync(`curl -s --noproxy '*' -H 'Content-Type: application/json' -d @- ${base}/mcp`, { input: body, encoding: 'utf8' });
  const res = JSON.parse(raw).result;
  if (res.isError) throw new Error(`${name}: ${res.content[0].text}`);
  return res.structuredContent;
};

(async () => {
  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const shot = async (file, locator) => {
    await page.waitForTimeout(400);
    if (locator) await page.locator(locator).first().scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${out}/${file}`, fullPage: false });
    console.log(`${file}\t${page.url().replace(base, '')}\t"${await page.title()}"`);
  };
  const expectText = async (text) => {
    if (!(await page.content()).includes(text)) throw new Error(`expected "${text}" on ${page.url()}`);
  };

  // 1. The design the chat agent started and submitted, waiting for a person.
  await page.goto(base + '/ui/id/INIT-001-design');
  await expectText('Started from the template by the chat agent.');
  await shot('01-design-waiting.png');

  // 2. The person approves it in the web UI.
  await page.getByRole('button', { name: 'Approve this document' }).click();
  await page.waitForLoadState('load');
  await expectText('Approved by sam.');
  await shot('02-design-approved.png');

  // 3. The chat agent writes the spec, adopts it, submits it and carries the
  //    person's approval; then plans a milestone, a roadmap and a checklist.
  const log = execSync(`bash ${here}/chat2.sh`, { encoding: 'utf8' });
  console.log(log);
  require('fs').writeFileSync(`${out}/chat2-output.txt`, log);

  // 4. The spec says who wrote it and who approved it.
  await page.goto(base + '/ui/id/FEAT-001-spec');
  await expectText('Written by the chat agent.');
  await expectText('Approved by a person, relayed by the chat agent');
  await shot('03-spec-written-by.png');

  // 5. The feature page, and its send screen: spec writing is skipped.
  await page.goto(base + '/ui/id/FEAT-001');
  await shot('04-feature.png');
  const feature = mcp('get_feature', { path: 'FEAT-001' });
  const fid = (await page.locator('input[name=id]').first().getAttribute('value'));
  await page.goto(base + '/ui/send?feature=' + fid);
  await expectText('Already written by the chat agent. This step is skipped.');
  await expectText('This step is skipped.');
  await shot('05-send-screen.png', 'table.table');

  // 6. The person presses Send. With no model, the plan writer's run fails
  //    at the provider; the mock-provider tests prove what a real model does.
  await page.getByRole('button', { name: /Send .*to development/ }).click();
  await page.waitForLoadState('load');
  await page.waitForTimeout(3000);
  await page.goto(base + '/ui/id/FEAT-001');
  await page.getByText('What happened at each step').click();
  await page.waitForTimeout(300);
  await expectText('by a person, relayed by the chat agent');
  await shot('06-sent-timeline.png', 'text=by a person, relayed by the chat agent');

  // 7. IDs in the plan section, on the checklist page and on the milestone.
  await page.goto(base + '/ui/project');
  await expectText('<span class="ident">MS-001</span>');
  await expectText('<span class="ident">RM-001</span>');
  await expectText('<span class="ident">CL-001</span>');
  await shot('07-plan-ids.png', 'text=The road to hello');
  await page.goto(base + '/ui/id/CL-001');
  await expectText('<span class="ident">CL-001</span>Launch jobs');
  await shot('08-checklist-id.png');
  await page.goto(base + '/ui/id/MS-001');
  await shot('09-milestone-items.png');

  // 8. The read tools, from chat: the timeline, and the failed plan run.
  const tl = mcp('get_timeline', { feature: 'FEAT-001' });
  const lines = tl.moments.map((m) => `${m.label} — ${m.who}`);
  let run = null;
  for (const m of tl.moments) for (const r of (m.runs || [])) run = run || r;
  let runOut = 'no run yet';
  if (run) {
    const ar = mcp('get_agent_run', { run_id: run.run_id });
    runOut = JSON.stringify({ what: ar.what, state: ar.state, conclusion: ar.conclusion, url: ar.url,
      transcript_turns: ar.transcript ? ar.transcript.turns.length : 0, tool_results: ar.transcript && ar.transcript.tool_results }, null, 2);
  }
  const read = `get_timeline FEAT-001:\n  ${lines.join('\n  ')}\n\nget_agent_run ${run ? run.run_id : ''}:\n${runOut}\n`;
  console.log(read);
  require('fs').writeFileSync(`${out}/read-tools-output.txt`, read);
  await browser.close();
})().catch((e) => { console.error(e); process.exit(1); });
