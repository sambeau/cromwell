// The SPEC-018 walkthrough (DoD 6). Run setup.sh first, then, from the
// repository root:
//   node docs/walkthrough-spec-018/walk.js docs/walkthrough-spec-018
// No AI provider is used. The browser plays the person; plain MCP JSON-RPC
// plays the chat agent; fake_provider.py answers the one agent run, a spec
// review, and logs what it was sent. Screenshots go in the directory given.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');
const fs = require('fs');

const base = 'http://127.0.0.1:8818';
const repo = '/var/tmp/m11demo';
const out = process.argv[2] || '.';
const DB = 'postgres://postgres@localhost:54329/m11_demo?sslmode=disable';

const mcp = (name, args) => {
  const body = JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } });
  const raw = execSync(`curl -s --noproxy '*' -H 'Content-Type: application/json' -d @- ${base}/mcp`, { input: body, encoding: 'utf8' });
  const res = JSON.parse(raw).result;
  if (res.isError) throw new Error(`${name}: ${res.content[0].text}`);
  return res.structuredContent;
};
const sql = (q) => execSync(`psql "${DB}" -Atc "${q}"`, { encoding: 'utf8' }).trim();
const read = (p) => fs.readFileSync(`${repo}/${p}`, 'utf8');
const write = (p, s) => fs.writeFileSync(`${repo}/${p}`, s);
const git = (args) => execSync(`git -C ${repo} ${args}`, { encoding: 'utf8' });

// fill writes a ruling and reason into a decision started from the template,
// as a person would in their editor, and fills the other placeholders.
const fill = (p, ruling, reason, context) => {
  let s = read(p);
  s = s.replace(/ruling: "\{\{[^}]*\}\}"/, `ruling: ${JSON.stringify(ruling)}`);
  s = s.replace(/reason: "\{\{[^}]*\}\}"/, `reason: ${JSON.stringify(reason)}`);
  s = s.replace(/\{\{[\s\S]*?\}\}/g, context || 'Written for the walkthrough.');
  write(p, s);
};

(async () => {
  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const shot = async (file, locator) => {
    await page.waitForTimeout(300);
    if (locator) await page.locator(locator).first().scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${out}/${file}`, fullPage: false });
    console.log(`${file}\t${page.url().replace(base, '')}\t"${await page.title()}"`);
  };
  const expectText = async (text) => {
    if (!(await page.content()).includes(text)) throw new Error(`expected "${text}" on ${page.url()}`);
  };
  // The forms are boosted, so a redirect can land after the click resolves.
  const editorPath = async () => {
    await page.waitForURL(/\/ui\/edit\//);
    await page.waitForLoadState('load');
    return decodeURIComponent(new URL(page.url()).pathname.replace('/ui/edit/', ''));
  };
  // click presses a button that submits a form, and waits for the page it
  // leads to; open presses one that opens a dialog.
  // The page's live event stream never lets the network go idle, so a
  // boosted form is given a moment to swap the page in.
  const click = async (name) => {
    await page.getByRole('button', { name }).first().click();
    await page.waitForTimeout(900);
    await page.waitForLoadState('load');
  };
  const open = async (name) => {
    await page.getByRole('button', { name }).first().click();
    await page.waitForTimeout(200);
  };

  // 1. This repository's DEC-001 to DEC-007, adopted as they are, through the
  //    web UI's adopt route. Each keeps its number and is told by its title.
  for (const f of fs.readdirSync(`${repo}/docs/decisions`).sort()) {
    const r = await page.request.post(base + '/ui/entity/adopt', {
      form: { file_path: `docs/decisions/${f}`, doc_type: 'decision', owner_type: 'project', state: 'approved' },
    });
    if (r.status() >= 400) throw new Error(`adopt ${f}: ${r.status()}`);
  }
  await page.goto(base + '/ui/decisions');
  await expectText('DEC-001: Server Language — Go');
  await expectText('No ruling is recorded; its title is surfaced.');
  await shot('01-viewer-adopted.png');

  // 2. The chat agent plans an initiative and a feature.
  mcp('create_initiative', { slug: 'auth', name: 'Authentication', design_document: false });
  mcp('create_feature', { initiative_path: 'auth', slug: 'login', name: 'Login form', design_document: false,
    description: 'People sign in with an email address and a password.' });

  // 3. The project conventions: started from the page, written, accepted.
  await page.goto(base + '/ui/decisions');
  await click('Start the conventions document');
  const convPath = await editorPath();
  await shot('02-conventions-editor.png');
  write(convPath, read(convPath).replace(/## Code[\s\S]*$/, '## Code\n\n- Errors are full sentences a person can act on.\n- No new dependency without a decision.\n\n## Tools\n\n- `go test ./...` must pass before anything is submitted.\n'));
  await page.goto(base + '/ui/d/' + convPath);
  await click('Submit for review');
  await click('Approve this document');
  await expectText('What agents are told');
  await shot('03-conventions-accepted.png', '#surfaced');

  // 4. A new decision, for the project. A 200-word ruling is refused.
  await page.goto(base + '/ui/decisions');
  await open('New decision…');
  await page.fill('#nd-title', 'Sessions live in Postgres');
  await shot('04-new-decision-dialog.png');
  await click('Start the decision');
  const d8 = await editorPath();
  await shot('05-decision-in-the-editor.png');
  fill(d8, Array(200).fill('word').join(' '), 'Restarts keep people signed in.');
  await page.goto(base + '/ui/d/' + d8);
  await click('Submit for review');
  await expectText('The ruling is 200 words; it can be at most 75');
  await shot('06-long-ruling-refused.png');
  fill(d8, 'Sessions are stored in Postgres, never in server memory.', 'Restarts keep people signed in.');
  write(d8, read(d8).replace(/ruling: ".*"/, 'ruling: "Sessions are stored in Postgres, never in server memory."'));
  await page.goto(base + '/ui/d/' + d8);
  await click('Submit for review');
  await expectText('A person decides whether to accept this decision.');
  await shot('07-decision-waiting.png');
  await click('Accept this decision');
  await expectText('Every agent working on its branch of the project is told this line:');
  await shot('08-decision-accepted.png', '#surfaced');

  // 5. An initiative's decision, which only its own branch is told.
  await page.goto(base + '/ui/decisions');
  await open('New decision…');
  await page.fill('#nd-title', 'Passwords are hashed with argon2id');
  await page.selectOption('#nd-owner', 'INIT-001');
  await click('Start the decision');
  const dAuth = await editorPath();
  fill(dAuth, 'Hash passwords with argon2id, with the library defaults.', 'It is the current recommendation.');
  await page.goto(base + '/ui/d/' + dAuth);
  await click('Submit for review');
  await click('Accept this decision');

  // 6. Supersede DEC-008 with a new decision, from its page.
  await page.goto(base + '/ui/d/' + d8);
  await open('Supersede with a new decision');
  await page.fill('#sup-title', 'Sessions live in signed cookies');
  await shot('09-supersede-dialog.png');
  await click('Start the new decision');
  const d10 = await editorPath();
  fill(d10, 'Sessions are signed cookies; the server keeps no session state.', 'Any server can answer any request.');
  await page.goto(base + '/ui/d/' + d10);
  await click('Submit for review');
  await expectText('Accepting it supersedes');
  await click('Accept this decision');
  await page.goto(base + '/ui/d/' + d8);
  await expectText('so agents are no longer told it');
  await shot('10-superseded-decision.png', '#surfaced');

  // 7. Amend DEC-010: an append-only revision, accepted by a person.
  await page.goto(base + '/ui/d/' + d10);
  await click('Append an amendment');
  const amend = await editorPath();
  await shot('11-amendment-skeleton.png');
  let a = read(amend);
  // Changing the accepted text is refused.
  write(amend, a.replace('Written for the walkthrough.', 'Rewritten.').replace(/\{\{[\s\S]*?\}\}/g, 'Cookies expire'));
  await page.goto(base + '/ui/d/' + amend);
  await click('Submit for review');
  await expectText('An accepted decision is never edited');
  await shot('12-edit-refused.png');
  write(amend, a.replace('{{what changed}}', 'cookies expire')
    .replace(/\{\{What the amendment[\s\S]*?\}\}/, 'Session cookies expire after twelve hours of inactivity. The ruling is restated to say so.')
    .replace(/ruling: ".*"/, 'ruling: "Sessions are signed cookies that expire after twelve idle hours; the server keeps no session state."'));
  await page.goto(base + '/ui/d/' + amend);
  await click('Submit for review');
  await click('Accept this decision');
  await expectText('Amendment 1 — cookies expire');
  await shot('13-amendment-accepted.png', '#surfaced');

  // 8. Record DEC-001's ruling: the one revision with nothing appended.
  await page.goto(base + '/ui/id/DEC-001');
  await click('Record its ruling');
  const rec = await editorPath();
  write(rec, read(rec)
    .replace(/ruling: "\{\{[^}]*\}\}"/, 'ruling: "The server, the CLI and all tooling are written in Go."')
    .replace(/reason: "\{\{[^}]*\}\}"/, 'reason: "One static binary, and the team\'s experience."'));
  await page.goto(base + '/ui/d/' + rec);
  await click('Submit for review');
  await click('Accept this decision');

  // 9. The viewer: every decision, what agents are told, and who superseded whom.
  await page.goto(base + '/ui/decisions?state=all');
  await expectText('Amendment 1 — cookies expire');
  await shot('14-viewer-all.png', '#decisions');
  await page.goto(base + '/ui/decisions?owner=INIT-001');
  await expectText('What a dispatch in INIT-001 Authentication is told');
  await shot('15-viewer-branch.png', '#branch');

  // 10. The chat agent writes the login spec and hands it in; the spec
  //     reviewer runs against the fake provider. Its transcript shows what it
  //     was told.
  const specPath = 'docs/work/INIT-001-auth/FEAT-001-spec.md';
  fs.mkdirSync(`${repo}/docs/work/INIT-001-auth`, { recursive: true });
  write(specPath, `---\ntitle: Login form\ntype: spec\nowner: auth/login\n---\n\n# Login form\n\n## Overview\n\nPeople sign in with an email address and a password.\n\n## Behaviour\n\nValid credentials start a session. Invalid credentials show an error and keep the email address.\n\n## Acceptance criteria\n\n- Valid credentials start a session\n- Invalid credentials show an error and keep the email address\n`);
  git(`add ${specPath}`);
  git(`commit -qm "the login spec, drafted in chat"`);
  mcp('adopt_document', { path: specPath, doc_type: 'spec', owner_type: 'feature', owner_path: 'FEAT-001' });
  mcp('submit_for_review', { document: 'FEAT-001-spec' });
  let run = '';
  for (let i = 0; i < 100 && !run; i++) {
    run = sql("SELECT id FROM dispatches WHERE purpose = 'review-spec' AND state = 'succeeded' LIMIT 1");
    if (!run) await page.waitForTimeout(200);
  }
  if (!run) throw new Error('the spec review never finished');
  await page.goto(base + '/ui/run/' + run);
  await page.locator('details.said').nth(1).evaluate((d) => { d.open = true; });
  await expectText('# Project decisions and conventions');
  await expectText('DEC-010: Sessions are signed cookies that expire after twelve idle hours');
  await expectText('DEC-009 (INIT-001 Authentication): Hash passwords with argon2id');
  await shot('16-run-what-it-was-told.png', 'details.said >> nth=1');
  const told = await page.locator('details.said').nth(1).locator('pre').innerText();
  fs.writeFileSync(`${out}/told.txt`, told);

  // What the provider was actually sent, from the fake's own log.
  const reqs = fs.readFileSync('/var/tmp/m11/requests.jsonl', 'utf8').trim().split('\n').map(JSON.parse);
  const last = reqs[reqs.length - 1];
  const user = last.messages[0].content.map((b) => b.text || '').join('');
  fs.writeFileSync(`${out}/provider-received.txt`, user);
  if (user !== told) console.log('note: the transcript and the request differ');

  const list = mcp('list_decisions', { owner: 'INIT-001' });
  fs.writeFileSync(`${out}/list_decisions.json`, JSON.stringify(list, null, 2));
  await browser.close();
})().catch((e) => { console.error(e); process.exit(1); });
