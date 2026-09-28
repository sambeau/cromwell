// The SPEC-015 browser walkthrough (DoD 3). Run setup.sh first, then:
//   node docs/walkthrough-spec-015/walk.js docs/walkthrough-spec-015
// No AI provider is used. Screenshots go in the directory given.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');

const base = 'http://127.0.0.1:8815';
const repo = '/var/tmp/m8demo';
const out = process.argv[2] || '.';
const sh = (cmd) => execSync(cmd, { cwd: repo, encoding: 'utf8' });

(async () => {
  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 860 } });
  const shot = async (file) => {
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${out}/${file}`, fullPage: false });
    console.log(`${file}\t${page.url().replace(base, '')}\t"${await page.title()}"`);
  };
  const menu = async (item) => {
    await page.locator('.page-head details.menu > summary').click();
    await page.getByRole('button', { name: item }).click();
  };

  // 1. Create an initiative from the project page, with its design.
  await page.goto(base + '/ui/project');
  await menu('New initiative');
  const ni = page.locator('#dlg-new-initiative');
  await ni.locator('input[name=name]').fill('Authentication');
  await ni.locator('input[name=slug]').fill('auth');
  await ni.locator('input[name=description]').fill('Signing people in and keeping them signed in.');
  await shot('01-new-initiative-dialog.png');
  await ni.getByRole('button', { name: 'Create' }).click();
  await page.waitForLoadState('load');
  await shot('02-initiative-created.png');

  // 2. The initiative's page: its ID, and its starter design as the body.
  await page.goto(base + '/ui/i/auth');
  await shot('03-initiative-page.png');

  // 3. A feature under it, with its design.
  await menu('New feature here');
  const nf = page.locator('#dlg-new-feature');
  await nf.locator('input[name=name]').fill('Login form');
  await nf.locator('input[name=slug]').fill('login');
  await nf.locator('input[name=description]').fill('An email address, a password and a button.');
  await nf.getByRole('button', { name: 'Create the feature' }).click();
  await page.waitForLoadState('load');
  await shot('04-feature-created.png');
  await page.goto(base + '/ui/f/auth/login');
  await shot('05-feature-page.png');
  console.log(sh('git log --format="%h %an: %s" -3'));
  console.log(sh('find docs/work -type f | sort'));

  // 4. Move the feature's design with git, and commit: the hook tells the
  // server, which follows the ID.
  sh('mkdir -p docs/designs && git mv docs/work/INIT-001-auth/FEAT-001-design.md docs/designs/login-form.md');
  sh('git commit -qm "Move the login design"');
  await page.waitForTimeout(1500);
  await page.goto(base + '/ui/f/auth/login');
  await shot('06-feature-after-move.png');
  await page.goto(base + '/ui/id/FEAT-001-design');
  await shot('07-design-at-its-new-path.png');
  await page.goto(base + '/ui/d/docs/work/INIT-001-auth/FEAT-001-design.md');
  console.log(`old address now lands on ${page.url().replace(base, '')}`);

  // 5. Adopt this repository's DEC-005 where it sits, as already approved.
  await page.goto(base + '/ui/project');
  await menu('Adopt a file…');
  const ad = page.locator('#dlg-adopt');
  await ad.locator('input[name=file_path]').fill('docs/decisions/DEC-005-the-orchestration-boundary.md');
  await ad.locator('select[name=doc_type]').selectOption('decision');
  await ad.locator('select[name=state]').selectOption('approved');
  await shot('08-adopt-dialog.png');
  await ad.getByRole('button', { name: 'Adopt the file' }).click();
  await page.waitForLoadState('load');
  await shot('09-adopted.png');
  await page.goto(base + '/ui/id/DEC-005');
  await shot('10-decision-page.png');
  console.log(sh('git show --stat --format="%h %an: %s" HEAD'));
  console.log(sh('head -8 docs/decisions/DEC-005-the-orchestration-boundary.md'));

  // 6. Adopt an older note on the initiative, as a draft.
  await page.goto(base + '/ui/i/auth');
  await menu('Adopt a file…');
  await ad.locator('input[name=file_path]').fill('docs/notes/session-tokens.md');
  await ad.locator('select[name=doc_type]').selectOption('note');
  await ad.getByRole('button', { name: 'Adopt the file' }).click();
  await page.waitForLoadState('load');
  await shot('11-note-adopted.png');

  // 7. IDs in lists, and by ID.
  await page.goto(base + '/ui/documents');
  await shot('12-documents.png');
  await page.goto(base + '/ui/id/feat-001');
  console.log(`/ui/id/feat-001 lands on ${page.url().replace(base, '')}`);
  await page.goto(base + '/ui');
  await shot('13-home.png');

  await browser.close();
})();
