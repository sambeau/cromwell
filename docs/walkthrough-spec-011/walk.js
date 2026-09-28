// SPEC-011 DoD 3: Send to development in the browser, with no AI provider.
// Run with NODE_PATH=$(npm root -g) node walk.js <outdir>
const { chromium } = require('playwright');
const { execSync } = require('child_process');

const BASE = 'http://127.0.0.1:8811';
const OUT = process.argv[2];
const DB = 'postgres://postgres@localhost:54329/m3_demo?sslmode=disable';
const sql = (q) => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
let n = 0;
const log = (...a) => console.log(...a);

(async () => {
  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium' });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  page.on('pageerror', e => log('PAGE ERROR', e.message));
  const shot = async (name, opts = {}) => {
    n++;
    const file = `${OUT}/${String(n).padStart(2, '0')}-${name}.png`;
    await page.screenshot({ path: file, ...opts });
    log('shot', file);
  };
  const settle = async () => { await page.waitForLoadState('load'); await page.waitForTimeout(600); };
  const menu = async (label) => {
    await page.click('details.menu > summary');
    await page.click(`.menu-list button:has-text("${label}")`);
  };
  const notice = async (text) => {
    await page.waitForSelector(`.banner:has-text("${text}")`, { timeout: 5000 });
    log('notice:', text);
  };
  const dispatches = () => sql('SELECT count(*) FROM dispatches');

  // --- The tree, made through the UI ---
  await page.goto(BASE + '/ui/project'); await settle();
  await menu('New initiative');
  await page.fill('#ni-name', 'Greetings'); await page.fill('#ni-slug', 'greet');
  await page.click('#dlg-new-initiative button[type=submit]');
  await notice('Initiative created');
  const feature = async (slug, name, desc) => {
    await page.goto(BASE + '/ui/i/greet'); await settle();
    await menu('New feature here');
    await page.fill('#nf-name', name); await page.fill('#nf-slug', slug);
    if (desc) await page.fill('#nf-desc', desc);
    await page.click('#dlg-new-feature button[type=submit]');
    await notice('Feature created');
  };
  await feature('time', 'Tell the time', 'Tell a program the current date and time, in UTC.');
  await feature('hello', 'Say hello', 'Greet a person by name.');
  await feature('later', 'Something later', '');

  // --- 1. Before the design is approved, Send refuses in G0's words ---
  await page.goto(BASE + '/ui/f/greet/time'); await settle();
  await page.locator('#why-send').waitFor();
  log('send reason (no design):', await page.locator('#why-send').innerText());
  await shot('send-refused-no-design');

  // --- 2. Attach the design; Submit it from its page; approve it ---
  await page.goto(BASE + '/ui/i/greet'); await settle();
  await menu('Attach a document');
  await page.fill('#att-path', 'docs/greet/design.md');
  await page.selectOption('#att-type', 'design');
  await page.click('#dlg-attach button[type=submit]');
  await settle();
  await page.goto(BASE + '/ui/d/docs/greet/design.md'); await settle();
  await shot('design-draft-submit');
  await page.click('#doc-actions button:has-text("Submit for review")');
  await notice('submitted for review');
  log('dispatches after submitting the design:', dispatches());
  await shot('design-waits-for-a-person');
  await page.click('button:has-text("Approve this document")');
  await notice('was approved');
  await page.waitForTimeout(1500);
  const afterApprove = dispatches();
  log('dispatches after approving the design:', afterApprove);
  if (afterApprove !== '0') throw new Error('approving a design dispatched something');
  await shot('design-approved-nothing-started');

  // --- 3. The undescribed feature still can't be sent, and says why ---
  await page.goto(BASE + '/ui/f/greet/later'); await settle();
  log('send reason (no description):', await page.locator('#why-send').innerText());
  await shot('send-refused-no-description');

  // --- 4. The feature can now be sent: the button, then the send screen ---
  // Spend the budget so the governor holds new work in the queue: with no AI
  // provider, a dispatch that ran would only fail, and a queued one lets
  // Withdraw be shown.
  sql("INSERT INTO dispatches (id, state, purpose, role, model, ref_type, ref_id, idempotency_key, cost_usd, finished_at) VALUES (gen_random_uuid(), 'succeeded', 'estimate', 'estimator', 'claude-sonnet-5', 'project', gen_random_uuid(), 'demo-spent', 100, now())");
  await page.goto(BASE + '/ui/f/greet/time'); await settle();
  await shot('send-button');
  await page.click('.page-head-actions a:has-text("Send to development")');
  await settle();
  await shot('send-screen-one-feature', { fullPage: true });
  await page.click('button[type=submit]:has-text("Send to development")');
  await notice('Sent to development: Tell the time');
  await page.waitForTimeout(800);
  log('write-spec dispatches:', sql("SELECT state || ' ' || COALESCE(queue_reason,'') FROM dispatches WHERE purpose = 'write-spec'"));
  await page.goto(BASE + '/ui/f/greet/time'); await settle();
  await shot('sent-mark-and-withdraw');

  // --- 5. Withdraw while it waits ---
  await page.click('button:has-text("Withdraw the send")');
  await notice('The send was withdrawn');
  log('write-spec after withdraw:', sql("SELECT state FROM dispatches WHERE purpose = 'write-spec'"));
  await shot('withdrawn');

  // --- 6. Several at once, from the initiative ---
  await page.goto(BASE + '/ui/i/greet'); await settle();
  await page.click('.page-head-actions a:has-text("Send features to development")');
  await settle();
  await shot('send-screen-initiative', { fullPage: true });

  // --- 7. Revise the approved design; Detach a stray draft ---
  await page.goto(BASE + '/ui/d/docs/greet/design.md'); await settle();
  await page.click('#doc-actions button:has-text("Revise")');
  await notice('A revision was opened');
  await shot('revision-opened');

  await page.goto(BASE + '/ui/i/greet'); await settle();
  await menu('Attach a document');
  await page.fill('#att-path', 'docs/greet/notes.md');
  await page.selectOption('#att-type', 'note');
  await page.click('#dlg-attach button[type=submit]');
  await settle();
  await page.goto(BASE + '/ui/d/docs/greet/notes.md'); await settle();
  await page.click('#doc-actions button:has-text("Detach")');
  await page.check('#dlg-detach input[name=confirm]');
  await shot('detach-confirm');
  await page.click('#dlg-detach button[type=submit]');
  await notice('was detached');
  await shot('detached');

  // --- 8. Start building, on a feature made ready in the database ---
  // Readiness needs an approved spec and plan, which need an AI provider; the
  // mock-provider suite covers that path. Here the state is set directly.
  sql("UPDATE features SET state = 'ready' WHERE slug = 'hello'");
  await page.goto(BASE + '/ui/f/greet/hello'); await settle();
  await shot('start-building');
  log('start building button:', await page.locator('.page-head-actions button').allInnerTexts());

  await browser.close();
})().catch(e => { console.error(e); process.exit(1); });
