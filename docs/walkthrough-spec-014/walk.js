// SPEC-014 DoD 2 and 3, in the browser. Run with NODE_PATH=$(npm root -g):
//   node walk.js <out-dir> a   — build a checklist, put it in a milestone, tick it
//   node walk.js <out-dir> b   — after mcp_walk.py: the relayed words, and shipping
//                                a milestone with a job still open
const { chromium } = require('playwright');
const { execSync } = require('child_process');

const BASE = 'http://127.0.0.1:8815';
const OUT = process.argv[2];
const PART = process.argv[3] || 'a';
const DB = 'postgres://postgres@localhost:54329/m5_demo?sslmode=disable';
let n = PART === 'a' ? 0 : 20;
const log = (...a) => console.log(...a);
const sql = (q) => execSync(`psql "${DB}" -Atqc "${q}"`).toString().trim();

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
  const settle = async () => { await page.waitForLoadState('load'); await page.waitForTimeout(500); };
  const menu = async (label) => {
    await page.click('details.menu > summary');
    await page.click(`.menu-list button:has-text("${label}")`);
  };
  const notice = async (text) => {
    await page.waitForSelector(`.banner:has-text("${text}")`, { timeout: 5000 });
    log('notice:', text);
  };
  const progress = async () => (await page.locator('.progress-legend').first().innerText()).replace(/\s+/g, ' ');

  if (PART === 'a') {
    // --- The tree: one initiative, one feature, made through the UI ---
    await page.goto(BASE + '/ui/project'); await settle();
    await menu('New initiative');
    await page.fill('#ni-name', 'Authentication'); await page.fill('#ni-slug', 'auth');
    await page.click('#dlg-new-initiative button[type=submit]');
    await notice('Initiative created: Authentication');
    await page.goto(BASE + '/ui/i/auth'); await settle();
    await menu('New feature here');
    await page.fill('#nf-name', 'Login form'); await page.fill('#nf-slug', 'login');
    await page.click('#dlg-new-feature button[type=submit]');
    await notice('Feature created: Login form');

    // --- 1. New checklist, beside New milestone and New roadmap (FR-3) ---
    await page.goto(BASE + '/ui/i/auth'); await settle();
    await page.locator('#planning').scrollIntoViewIfNeeded();
    await page.click('#planning button:has-text("New checklist")');
    await page.fill('#nc-name', 'Launch paperwork');
    await page.fill('#nc-desc', 'The things only a person can do before sign-in goes live.');
    await shot('new-checklist-dialog');
    await page.click('#dlg-new-checklist button[type=submit]');
    await notice('Checklist created: Launch paperwork');
    await page.locator('#planning').scrollIntoViewIfNeeded();
    await shot('plan-with-checklist');

    // --- 2. Jobs added, changed and reordered in the editor (FR-5) ---
    await page.click('#planning button[aria-label="Edit the checklist Launch paperwork"]');
    await page.waitForSelector('#dlg-checklist[open]');
    for (const [title, note] of [
      ['Get the API key from the identity provider', ''],
      ['Sign the data processing agreement', 'Legal have the draft.'],
      ['Choose the sign-in button icon', ''],
    ]) {
      await page.fill('#dlg-checklist input[name=title]:not([type=hidden]) >> nth=-1', title);
      await page.fill('#dlg-checklist input[name=note] >> nth=-1', note);
      await page.click('#dlg-checklist button:has-text("Add it")');
      await page.waitForSelector(`#checklist-editor .banner:has-text("Added: ${title}")`);
    }
    await page.click('#dlg-checklist button[aria-label="Move Choose the sign-in button icon up"]');
    await page.waitForSelector('#checklist-editor .banner:has-text("is now number 2")');
    await shot('checklist-editor');
    await page.click('#dlg-checklist .modal-foot button:has-text("Done")');
    await settle();

    // --- 3. A milestone holding the feature and the checklist (FR-6) ---
    await page.click('#planning button:has-text("New milestone")');
    await page.fill('#nm-name', 'Sign-in beta');
    await page.click('#dlg-new-milestone button[type=submit]');
    await notice('Milestone created: Sign-in beta');
    await page.click('#planning button[aria-label="Edit the milestone Sign-in beta"]');
    await page.waitForSelector('#dlg-milestone[open]');
    await page.click('#member-candidates button[aria-label="Add Login form"]');
    await page.waitForSelector('#milestone-editor .banner:has-text("Login form was added")');
    await page.click('#member-candidates button[aria-label="Add Launch paperwork"]');
    await page.waitForSelector('#milestone-editor .banner:has-text("Launch paperwork was added")');
    await shot('milestone-editor-with-checklist');
    await page.click('#dlg-milestone .modal-foot button:has-text("Done")');
    await settle();

    // Login form is finished. Nothing in this walkthrough runs an agent, so
    // the script marks it done in the database, as the M4 walkthrough did.
    sql("UPDATE features SET state = 'done' WHERE slug = 'login'");
    const mID = sql("SELECT id FROM milestones WHERE name = 'Sign-in beta'");
    const cID = sql("SELECT id FROM checklists WHERE name = 'Launch paperwork'");

    // --- 4. Not complete: the feature is done, the checklist isn't (FR-2) ---
    await page.goto(BASE + '/ui/m/' + mID); await settle();
    log('milestone before ticking:', await progress());
    await shot('milestone-not-complete');

    // --- 5. The checklist page: plain checkboxes (FR-4) ---
    await page.goto(BASE + '/ui/c/' + cID); await settle();
    await shot('checklist-page-open');
    // Tick one with a note, through the disclosure.
    const withNote = page.locator('li.job', { hasText: 'Get the API key' });
    await withNote.locator('summary:has-text("Tick with a note…")').click();
    await withNote.locator('input[name=note]').fill('Stored in the team vault under "IdP prod".');
    await shot('tick-with-a-note');
    await withNote.locator('button:has-text("Tick it")').click();
    await notice('Ticked: Get the API key from the identity provider.');
    // Tick one by pressing its box.
    await page.click('button[aria-label="Tick Choose the sign-in button icon"]');
    await notice('Ticked: Choose the sign-in button icon.');
    await shot('checklist-two-ticked');

    await page.goto(BASE + '/ui/m/' + mID); await settle();
    log('milestone with one job open:', await progress());

    // --- 6. The last job: the milestone is complete ---
    await page.goto(BASE + '/ui/c/' + cID); await settle();
    await page.click('button[aria-label="Tick Sign the data processing agreement"]');
    await notice('Ticked: Sign the data processing agreement.');
    await shot('checklist-done');
    await page.goto(BASE + '/ui/m/' + mID); await settle();
    log('milestone after ticking:', await progress());
    await shot('milestone-complete');
  } else {
    // --- Part B, after mcp_walk.py ---
    const cID = sql("SELECT id FROM checklists WHERE name = 'Store listing'");
    const gaID = sql("SELECT id FROM milestones WHERE name = 'Sign-in GA'");

    // 7. The relayed tick shows the person's words (SD-8).
    await page.goto(BASE + '/ui/c/' + cID); await settle();
    await shot('relayed-tick-on-the-page');

    // 8. Shipping with a job still open records it as not shipped (DoD 3).
    await page.goto(BASE + '/ui/m/' + gaID); await settle();
    log('GA before shipping:', await progress());
    await page.click('button:has-text("Edit this milestone")');
    await page.waitForSelector('#dlg-milestone[open]');
    await page.locator('#dlg-milestone h3:has-text("Mark as shipped")').scrollIntoViewIfNeeded();
    await shot('ship-with-a-job-open');
    await page.click('#dlg-milestone form[action="/ui/milestone/lock"] button');
    await page.waitForSelector('#milestone-editor .banner:has-text("marked as shipped")');
    log('notice:', (await page.locator('#milestone-editor .banner').first().innerText()).trim());
    await shot('shipped-with-a-job-open');
    log('not_shipped:', sql("SELECT payload->'not_shipped' FROM audit_events WHERE kind = 'milestone.locked'"));
  }
  await browser.close();
})().catch(e => { console.error(e); process.exit(1); });
