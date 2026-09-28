// SPEC-010 DoD 2: build a two-milestone roadmap for an initiative entirely in
// the browser. Run with NODE_PATH=$(npm root -g).
const { chromium } = require('playwright');
const { execSync } = require('child_process');

const BASE = 'http://127.0.0.1:8810';
const OUT = process.argv[2];
const DB = 'postgres://postgres@localhost:54329/m4_demo?sslmode=disable';
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

  // --- The tree to plan over, made through the UI as well ---
  await page.goto(BASE + '/ui/project'); await settle();
  for (const [slug, name] of [['auth', 'Authentication'], ['billing', 'Billing']]) {
    await menu('New initiative');
    await page.fill('#ni-name', name); await page.fill('#ni-slug', slug);
    await page.click('#dlg-new-initiative button[type=submit]');
    await notice('Initiative created: ' + name);
  }
  const feature = async (init, slug, name) => {
    await page.goto(BASE + '/ui/i/' + init); await settle();
    await menu('New feature here');
    await page.fill('#nf-name', name); await page.fill('#nf-slug', slug);
    await page.click('#dlg-new-feature button[type=submit]');
    await notice('Feature created: ' + name);
  };
  await feature('auth', 'login', 'Login form');
  await feature('auth', 'passkeys', 'Passkeys');
  await feature('billing', 'invoices', 'Invoices');

  // --- 1. The initiative's own plan, empty ---
  await page.goto(BASE + '/ui/i/auth'); await settle();
  await page.locator('#planning').scrollIntoViewIfNeeded();
  await shot('empty-plan');

  // --- 2. Create two milestones and a roadmap from the initiative's page ---
  await page.click('#planning button:has-text("New milestone")');
  await page.fill('#nm-name', 'Auth beta');
  await page.fill('#nm-date', '2026-11-30');
  await page.fill('#nm-desc', 'People can sign in with an email address and a password.');
  await shot('new-milestone-dialog');
  await page.click('#dlg-new-milestone button[type=submit]');
  await notice('Milestone created: Auth beta');
  await page.click('#planning button:has-text("New milestone")');
  await page.fill('#nm-name', 'Auth GA');
  await page.click('#dlg-new-milestone button[type=submit]');
  await notice('Milestone created: Auth GA');
  await page.click('#planning button:has-text("New roadmap")');
  await page.fill('#nr-name', 'Auth plan');
  await page.click('#dlg-new-roadmap button[type=submit]');
  await notice('Roadmap created: Auth plan');
  await page.locator('#planning').scrollIntoViewIfNeeded();
  await shot('plan-created');

  // --- 3. Add from the member's end: the feature's own page ---
  await page.goto(BASE + '/ui/f/auth/login'); await settle();
  await menu('Add to a milestone…');
  await page.selectOption('#atm-select', { label: 'Auth beta' });
  await shot('add-from-member');
  await page.click('#dlg-milestones button[type=submit]');
  await notice('Login form was added to Auth beta.');
  await shot('member-rail');

  // --- 4. Add from the milestone's end: the editor, subtree then search ---
  await page.goto(BASE + '/ui/i/auth'); await settle();
  await page.click('button[aria-label="Edit the milestone Auth GA"]');
  await page.waitForSelector('#dlg-milestone[open]');
  await shot('editor-subtree');
  await page.click('#member-candidates button[aria-label="Add Passkeys"]');
  await notice('Passkeys was added to Auth GA.');
  await page.fill('#dlg-milestone input[type=search]', 'invoi');
  await page.waitForSelector('#member-candidates button[aria-label="Add Invoices"]');
  await shot('editor-search');
  await page.click('#member-candidates button[aria-label="Add Invoices"]');
  await notice('Invoices was added to Auth GA.');
  // Auth GA also delivers everything in Auth beta.
  await page.fill('#dlg-milestone input[type=search]', 'beta');
  await page.waitForSelector('#member-candidates button[aria-label="Add Auth beta"]');
  await page.click('#member-candidates button[aria-label="Add Auth beta"]');
  await notice('Auth beta was added to Auth GA.');
  await shot('editor-composed');
  // Take Invoices back out, with a reason, from the milestone's end.
  await page.click('#milestone-editor summary[aria-label="Take Invoices out"]');
  await page.fill('#milestone-editor details[open] input[name=reason]', 'Billing ships in its own release.');
  await page.click('#milestone-editor details[open] button[type=submit]');
  await notice('Invoices was taken out of Auth GA.');
  await page.click('#milestone-editor button:has-text("Done")');
  await settle(); // closing after a change reloads the page (FR-6.4)
  log('dialog open after Done:', await page.locator('#dlg-milestone').count());

  // --- 5. Order the roadmap ---
  await page.click('button[aria-label="Edit the roadmap Auth plan"]');
  await page.waitForSelector('#dlg-roadmap[open]');
  await page.selectOption('#dlg-roadmap select[name=milestone_id]', { label: 'Auth GA' });
  await page.click('#roadmap-editor button:has-text("Place it on the roadmap")');
  await notice('Auth GA is now number 1');
  await page.selectOption('#dlg-roadmap select[name=milestone_id]', { label: 'Auth beta' });
  await page.click('#roadmap-editor button:has-text("Place it on the roadmap")');
  await notice('Auth beta is now number 2');
  await shot('roadmap-placed');
  await page.click('button[aria-label="Move Auth beta up"]');
  await notice('Auth beta is now number 1');
  await shot('roadmap-reordered');
  await page.click('#roadmap-editor button:has-text("Done")');
  await settle();

  // --- 6. Lock: refused by G4, then confirmed ---
  await page.click('button[aria-label="Edit the milestone Auth beta"]');
  await page.waitForSelector('#dlg-milestone[open]');
  await page.locator('#milestone-editor .action-reason').scrollIntoViewIfNeeded();
  await shot('lock-refused');
  await page.click('#milestone-editor button:has-text("Done")');
  // No AI provider can build the feature, so it is marked done directly.
  execSync(`psql "${DB}" -qc "UPDATE features SET state = 'done' WHERE slug = 'login'"`);
  log('marked auth/login done in the database');
  await page.goto(BASE + '/ui/i/auth'); await settle();
  await page.click('button[aria-label="Edit the milestone Auth beta"]');
  await page.waitForSelector('#dlg-milestone[open]');
  await page.click('#milestone-editor summary:has-text("Lock this milestone")');
  await page.locator('#milestone-editor button:has-text("Lock it permanently")').scrollIntoViewIfNeeded();
  await shot('lock-confirm');
  await page.click('#milestone-editor button:has-text("Lock it permanently")');
  await notice('This milestone is now locked.');
  await shot('locked');
  await page.click('#milestone-editor button:has-text("Done")');
  await settle();

  // --- 7. The result, on the owner's page and the roadmap's ---
  await page.locator('#planning').scrollIntoViewIfNeeded();
  await shot('plan-final');
  await page.click('#planning a:has-text("Auth plan")'); await settle();
  await shot('roadmap-page');

  await browser.close();
})().catch(e => { console.error(e); process.exit(1); });
