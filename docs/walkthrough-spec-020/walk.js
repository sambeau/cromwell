// The SPEC-020 walkthrough (DoD 3). Run setup.sh first, then:
//   node docs/walkthrough-spec-020/walk.js docs/walkthrough-spec-020
// No AI provider is used. The browser plays the person; the chat agent's
// calls are plain MCP JSON-RPC, logged to chat-output.txt. Screenshots go in
// the directory given.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const base = 'http://127.0.0.1:8820';
const db = 'postgres://postgres@localhost:54329/m13_demo?sslmode=disable';
const out = process.argv[2] || '.';
const log = [];

const mcp = (name, args, pick) => {
  const body = JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } });
  const raw = execSync(`curl -s --noproxy '*' -H 'Content-Type: application/json' -d @- ${base}/mcp`, { input: body, encoding: 'utf8' });
  const res = JSON.parse(raw).result;
  if (res.isError) {
    log.push(`» ${name} ${JSON.stringify(args)}\n  refused: ${res.content[0].text}`);
    return null;
  }
  const r = res.structuredContent;
  log.push(`» ${name} ${JSON.stringify(args)}\n  ${JSON.stringify(pick ? pick(r) : r, null, 1).replace(/\n\s*/g, ' ')}`);
  return r;
};
const sql = (q) => execSync(`psql "${db}" -tAc "${q}"`, { encoding: 'utf8' }).trim();
const sh = (cmd, cwd) => execSync(cmd, { cwd, encoding: 'utf8' });
const waitFor = async (what, fn, ms = 20000) => {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (fn()) return;
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error('timed out waiting for ' + what);
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
  const expectText = async (text) => {
    try {
      await page.waitForFunction((t) => document.body.innerText.includes(t), text, { timeout: 8000 });
    } catch (e) {
      await page.screenshot({ path: `${out}/failed.png` });
      throw new Error(`expected "${text}" on ${page.url()}`);
    }
  };

  // 1. A person presses Start building. The budget is spent, so each task's
  //    implement dispatch waits in the queue.
  await page.goto(base + '/ui/id/FEAT-001');
  await page.getByRole('button', { name: 'Start building' }).first().click();
  await expectText('Tasks in this feature');
  await waitFor('implement dispatches queued', () =>
    sql("SELECT count(*) FROM dispatches WHERE purpose = 'implement-task' AND state = 'queued'") === '2');
  await page.goto(base + '/ui/id/FEAT-001');
  await shot('01-building.png', 'text=Tasks in this feature');

  // 2. The chat agent finds a task, claims it, works in the working copy and
  //    submits. The claim cancels the task's queued dispatch.
  mcp('get_feature', { path: 'FEAT-001' }, (r) => r.tasks.map((t) => ({ id: t.id, state: t.state, claimable: t.claimable, executor: t.executor.sentence })));
  const claim = mcp('claim_task', { task: 'FEAT-001-T01' }, (r) => ({ working_copy: r.working_copy, rules: r.rules.slice(0, 3), expires: r.expires, next: r.next }));
  mcp('claim_task', { task: 'FEAT-001-T02' });
  const wc = claim.working_copy.path;
  fs.writeFileSync(path.join(wc, 'greet.go'), `package main

// Greeting says hello by the hour of the day.
func Greeting(hour int) string {
	switch {
	case hour < 12:
		return "Good morning"
	case hour < 18:
		return "Good afternoon"
	}
	return "Good evening"
}
`);
  mcp('submit_task', { task: 'FEAT-001-T01', summary: 'Added Greeting(hour), with the three greetings by the hour.' },
    (r) => ({ state: r.task.state, review: r.review, commit: r.commit, next: r.next }));
  mcp('submit_task', { task: 'FEAT-001-T01', summary: 'Again.' });
  log.push('cancelled dispatches: ' + sql("SELECT count(*) FROM dispatches WHERE purpose = 'implement-task' AND state = 'cancelled'"));
  log.push('code review queued with: ' + sql("SELECT model FROM dispatches WHERE purpose = 'review-code'"));

  await page.goto(base + '/ui/id/FEAT-001-T01');
  await expectText('the chat agent');
  await shot('02-chat-task.png');
  await page.goto(base + '/ui/id/FEAT-001');
  await shot('03-feature-tasks.png', 'text=Tasks in this feature');

  // 3. A person claims T02 in the web UI.
  await page.goto(base + '/ui/id/FEAT-001-T02');
  await shot('04-claimable.png', 'text=Claiming this task');
  await page.getByRole('button', { name: 'Claim this task' }).click();
  await expectText('What you did');
  await shot('05-person-claim.png', 'text=Claiming this task');

  // 4. Nobody touches it for a day (the claim is backdated), and the claim
  //    sweep asks whether anyone is still working on it.
  sql("UPDATE work_claims SET last_activity_at = now() - interval '25 hours', claimed_at = now() - interval '26 hours' WHERE state = 'open'");
  await waitFor('the stale-claim question', () => sql("SELECT count(*) FROM checkpoints WHERE kind = 'claim-stale' AND state = 'pending'") === '1');
  await page.goto(base + '/ui/inbox');
  await expectText('Keep the claim');
  await shot('06-still-working.png', 'text=Keep the claim');
  await page.getByRole('button', { name: 'Keep the claim' }).first().click();
  await waitFor('the answer', () => sql("SELECT count(*) FROM checkpoints WHERE kind = 'claim-stale' AND state = 'pending'") === '0');

  // 5. The person does the work and submits it from the page.
  fs.writeFileSync(path.join(wc, 'home.go'), `package main

import (
	"fmt"
	"time"
)

func main() { fmt.Println(Greeting(time.Now().Hour())) }
`);
  await page.goto(base + '/ui/id/FEAT-001-T02');
  await page.fill('#claim-summary', 'The home page prints the greeting for the current hour.');
  await page.getByRole('button', { name: 'Submit for code review' }).click();
  await expectText('code reviewer is reviewing it');
  await shot('07-person-submitted.png', 'text=Claiming this task');

  // 6. Someone commits on the feature's branch with no claim. The branch
  //    watch notices.
  fs.writeFileSync(path.join(wc, 'README.md'), 'Greetings.\n');
  sh(`git add README.md && git -c user.name="Sam Phillips" -c user.email=sam@example.com commit -qm "quick readme"`, wc);
  await waitFor('the unclaimed-commit notice', () => sql("SELECT count(*) FROM checkpoints WHERE kind = 'unclaimed-commit' AND state = 'pending'") === '1');
  await page.goto(base + '/ui/inbox');
  await expectText("I've seen this");
  await shot('08-unclaimed-commit.png', "text=I've seen this");

  // 7. The feature's timeline shows the claims.
  await page.goto(base + '/ui/id/FEAT-001');
  await expectText('Claimed by the chat agent');
  await shot('09-timeline.png', 'text=Claimed by the chat agent');

  // 8. The chat agent still can't judge: the tools aren't there.
  for (const name of ['approve_task', 'release_task', 'verify_feature', 'submit_implementation']) {
    const body = JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: {} } });
    const raw = JSON.parse(execSync(`curl -s --noproxy '*' -H 'Content-Type: application/json' -d @- ${base}/mcp`, { input: body, encoding: 'utf8' }));
    log.push(`» ${name}\n  ${raw.error ? 'error ' + raw.error.code + ': ' + raw.error.message : 'callable!'}`);
  }

  fs.writeFileSync(`${out}/chat-output.txt`, log.join('\n') + '\n');
  await browser.close();
})().catch((e) => { fs.writeFileSync(`${out}/chat-output.txt`, log.join('\n') + '\n'); console.error(e); process.exit(1); });
