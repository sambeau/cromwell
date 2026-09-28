// The SPEC-016 browser walkthrough (DoD 3). Run setup.sh first, then:
//   node docs/walkthrough-spec-016/walk.js docs/walkthrough-spec-016
// No AI provider is used. Screenshots go in the directory given.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
const { execSync } = require('child_process');
const fs = require('fs');

const base = 'http://127.0.0.1:8816';
const repo = '/var/tmp/m9demo';
const out = process.argv[2] || '.';
const sh = (cmd) => execSync(cmd, { cwd: repo, encoding: 'utf8' });
const design = 'docs/work/INIT-001-auth/INIT-001-design.md';

(async () => {
  const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', args: ['--no-proxy-server'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const dialogs = [];
  page.on('dialog', async (d) => { dialogs.push(d.message()); await d.accept(); });
  const shot = async (file, full = false) => {
    await page.waitForTimeout(500);
    await page.screenshot({ path: `${out}/${file}`, fullPage: full });
    console.log(`${file}\t${page.url().replace(base, '')}\t"${await page.title()}"`);
  };
  const menu = async (item) => {
    await page.locator('.page-head details.menu > summary').click();
    await page.getByRole('button', { name: item }).click();
  };
  const text = page.locator('#ed-text');

  // 0. An initiative with its starter design, as M8 made it.
  await page.goto(base + '/ui/project');
  await menu('New initiative');
  const ni = page.locator('#dlg-new-initiative');
  await ni.locator('input[name=name]').fill('Authentication');
  await ni.locator('input[name=slug]').fill('auth');
  await ni.locator('input[name=description]').fill('Signing people in and keeping them signed in.');
  await ni.getByRole('button', { name: 'Create' }).click();
  await page.waitForLoadState('load');

  // 1. The design's page offers Edit.
  await page.goto(base + '/ui/d/' + design);
  await shot('01-document-page-edit-button.png');

  // 2. The editor: the whole file, the state, and the preview.
  await page.getByRole('link', { name: 'Edit', exact: true }).click();
  await page.waitForSelector('#ed-text');
  await shot('02-editor-opened.png');

  // 3. Fill in the starter's placeholders, as a person would, and watch the
  // preview follow.
  const answers = [
    'People sign in with an email address and a password, and stay signed in for a day.',
    'One sign-in form, one session cookie, and a sign-out link on every page.',
    'Passwords are hashed with Argon2id. Sessions are stored server-side.',
    'Single sign-on and two-factor sign-in come later.',
  ];
  let n = 0;
  const filled = (await text.inputValue())
    .replace(/\{\{[^}]*\}\}/g, () => answers[Math.min(n++, answers.length - 1)]);
  await text.fill(filled);
  await text.press('End');
  await page.waitForTimeout(900);
  await shot('03-editing-with-preview.png');

  // 4. Save & commit, and show the commit.
  await page.locator('#ed-subject').fill('Fill in the authentication design');
  await page.getByRole('button', { name: 'Save & commit' }).click();
  await page.waitForSelector('.banner--success');
  await shot('04-saved-and-committed.png');
  console.log(sh('git log -1 --format="%h  author: %an <%ae>  committer: %cn <%ce>%n    %s%n%n%b"'));
  console.log(sh('git show --stat --format= HEAD'));

  // 5. Open the editor again, type, and meanwhile change the file in a shell.
  await page.goto(base + '/ui/edit/' + design);
  await text.press('Control+End');
  await text.type('\nA sentence typed in the browser.\n');
  sh(`python3 -c "p='${design}'; s=open(p).read(); open(p,'w').write(s.replace('stay signed in for a day', 'stay signed in for a week', 1) + '\\nA line added in vim.\\n')"`);
  // The freshness check notices within ten seconds.
  await page.waitForSelector('#edit-fresh .banner--warning', { timeout: 15000 });
  await shot('05-changed-on-disk-warning.png');

  // 6. Save anyway: it stops, and shows both sides.
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForSelector('#edit-conflict');
  await shot('06-conflict.png');
  await page.locator('#edit-conflict').screenshot({ path: `${out}/07-conflict-diffs.png` });
  console.log('07-conflict-diffs.png\t(the conflict panel)');
  console.log('the file on disk still ends:\n' + sh(`tail -3 ${design}`));

  // 7. Copy your text, then Reload from disk. The editor asks first, because
  // the text isn't saved.
  await page.getByRole('button', { name: 'Copy your text' }).click();
  await text.press('End');
  await text.type(' More.');
  await page.getByRole('link', { name: 'Reload from disk' }).click();
  await page.waitForSelector('#ed-text');
  console.log('the leave question asked: ' + JSON.stringify(dialogs));
  await shot('08-reloaded.png');

  // 8. Refusing to break the ID.
  const broken = (await text.inputValue()).replace('id: INIT-001-design\n', '');
  await text.fill(broken);
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForSelector('.banner--error');
  await shot('09-id-refused.png');

  // 9. Commit the vim change, submit the design and approve it.
  sh(`git commit -qam "A week, not a day"`);
  await page.waitForTimeout(800);
  await page.goto(base + '/ui/d/' + design);
  await page.getByRole('button', { name: 'Submit for review' }).click();
  await page.waitForLoadState('load');
  await page.getByRole('button', { name: 'Approve this document' }).click();
  await page.waitForLoadState('load');
  await page.goto(base + '/ui/d/' + design);
  await shot('10-approved-design-page.png');

  // 10. Edit on an approved design: it starts a revision, and says so.
  await page.getByRole('link', { name: 'Edit — starts a revision' }).click();
  await page.waitForSelector('#edit-revise');
  await shot('11-approved-starts-a-revision.png');
  await page.getByRole('button', { name: 'Start a revision and edit it' }).click();
  await page.waitForSelector('#ed-text');
  await shot('12-successor-editor.png');

  // 11. Edit the successor and Save & commit it.
  await text.press('Control+End');
  await text.type('\n## Revised\n\nSessions now last a week.\n');
  await page.getByRole('button', { name: 'Save & commit' }).click();
  await page.waitForSelector('.banner--success');
  await shot('13-successor-saved.png');
  console.log(sh('git log -1 --format="%h  author: %an  committer: %cn%n    %s" && git show --stat --format= HEAD'));

  // 12. The original's page points to the open revision.
  await page.goto(base + '/ui/d/' + design);
  await shot('14-original-points-to-revision.png');

  // 13. Approve the revision: it takes the original's place.
  await page.goto(base + '/ui/d/' + design.replace('.md', '.rev.md'));
  await page.getByRole('button', { name: 'Submit for review' }).click();
  await page.waitForLoadState('load');
  await page.getByRole('button', { name: 'Approve this document' }).click();
  await page.waitForLoadState('load');
  await page.waitForTimeout(800);
  await page.goto(base + '/ui/d/' + design);
  await shot('15-revision-approved.png');
  console.log(sh('git log -3 --format="%h %an: %s" && git show --stat --format= HEAD && git status --short'));

  await browser.close();
})().catch((e) => { console.error(e); process.exit(1); });
