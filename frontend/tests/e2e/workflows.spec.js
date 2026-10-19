import { test, expect } from '@playwright/test';

async function createNote(request, path, content) {
  const created = await request.post('/api/notes', { data: { path } });
  expect(created.status()).toBe(201);
  if (content !== undefined) {
    const note = await (await request.get(`/api/note/${encodeURIComponent(path)}`)).json();
    const saved = await request.post(`/api/note/${encodeURIComponent(path)}`, { headers: { 'If-Match': note.revision, 'Content-Type': 'text/plain' }, data: content });
    expect(saved.ok()).toBeTruthy();
  }
}

test('creates a missing note and saves edits', async ({ page, request }) => {
  await page.goto('/note/New%20browser%20note');
  await page.getByRole('button', { name: 'Create this note' }).click();
  const editor = page.getByRole('textbox', { name: 'Note markdown' });
  await expect(editor).toBeVisible();
  await editor.fill('# Created in browser');
  await expect(page.getByRole('status')).toHaveText('Saved');
  const result = await (await request.get('/api/note/New%20browser%20note')).json();
  expect(result.content).toBe('# Created in browser');
});

test('navigation cannot autosave A content into B', async ({ page, request }) => {
  await page.goto('/note/A');
  await page.getByTitle('Edit mode', { exact: true }).click();
  await page.getByRole('textbox', { name: 'Note markdown' }).fill('A changed immediately before navigation');
  await page.locator('a[data-path="B"]').click();
  await expect(page.locator('[data-note-path="B"]')).toBeVisible();
  await expect.poll(async () => (await (await request.get('/api/note/A')).json()).content).toBe('A changed immediately before navigation');
  const b = await (await request.get('/api/note/B')).json();
  expect(b.content).toContain('Original B');
  expect(b.content).not.toContain('A changed');
});

test('stale note loads cannot replace a newer navigation', async ({ page }) => {
  await page.route('**/api/note/A', async route => { await new Promise(done => setTimeout(done, 800)); await route.continue().catch(() => {}); });
  await page.goto('/note/A');
  await page.locator('a[data-path="B"]').click();
  await expect(page.locator('[data-note-path="B"]')).toBeVisible();
  await page.waitForTimeout(1000);
  await expect(page.locator('[data-note-path="B"]')).toBeVisible();
  await expect(page.locator('.breadcrumbs .current')).toHaveText('B');
});

test('retains a conflicting draft and permits explicit reload', async ({ page, request }) => {
  await createNote(request, 'Conflict', 'original');
  await page.goto('/note/Conflict');
  await page.getByTitle('Edit mode', { exact: true }).click();
  let releaseSave;
  let saveStarted;
  const intercepted = new Promise(done => { saveStarted = done; });
  const gate = new Promise(done => { releaseSave = done; });
  await page.route('**/api/note/Conflict', async route => {
    if (route.request().method() === 'POST') { saveStarted(); await gate; }
    await route.continue();
  });
  await page.getByRole('textbox', { name: 'Note markdown' }).fill('local draft');
  await intercepted;
  const current = await (await request.get('/api/note/Conflict')).json();
  expect((await request.post('/api/note/Conflict', { headers: { 'If-Match': current.revision }, data: 'external edit' })).ok()).toBeTruthy();
  releaseSave();
  await expect(page.getByRole('status')).toHaveText('Conflict — draft retained');
  expect((await (await request.get('/api/note/Conflict')).json()).content).toBe('external edit');
  page.on('dialog', dialog => dialog.accept());
  await page.getByRole('button', { name: 'Discard draft and reload' }).click();
  await expect(page.getByRole('textbox', { name: 'Note markdown' })).toHaveValue('external edit');
});

test('renames and deletes notes through file actions', async ({ page, request }) => {
  await createNote(request, 'RenameMe', 'keep');
  await page.goto('/');
  await page.locator('a[data-path="RenameMe"]').click({ button: 'right' });
  page.once('dialog', dialog => dialog.accept('Renamed'));
  await page.getByRole('button', { name: 'Rename', exact: true }).click();
  await expect(page.locator('a[data-path="Renamed"]')).toBeVisible();
  expect((await request.get('/api/note/RenameMe')).status()).toBe(404);
  await page.locator('a[data-path="Renamed"]').click({ button: 'right' });
  page.once('dialog', dialog => dialog.accept());
  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  await expect(page.locator('a[data-path="Renamed"]')).toHaveCount(0);
  expect((await request.get('/api/note/Renamed')).status()).toBe(404);
});

test('downloads folders and rejects hostile rendering and origins', async ({ page, request }) => {
  await createNote(request, 'Download/one', 'download');
  const zip = await request.get('/api/download?path=Download');
  expect(zip.headers()['content-type']).toContain('application/zip');
  expect((await zip.body()).subarray(0, 2).toString()).toBe('PK');
  await createNote(request, 'Unsafe', '<img src=x onerror="window.hacked=true"><script>window.hacked=true</script> [[B]]');
  await page.goto('/note/Unsafe');
  await expect(page.locator('.markdown-content a.wikilink')).toBeVisible();
  expect(await page.evaluate(() => window.hacked)).toBeUndefined();
  expect((await request.post('/api/notes', { headers: { Origin: 'https://evil.example' }, data: { path: 'bad' } })).status()).toBe(403);
});

test('moving a dirty note flushes its own draft and updates the active route', async ({ page, request }) => {
  await createNote(request, 'MoveMe', 'original');
  expect((await request.post('/api/folders', { data: { path: 'Target' } })).status()).toBe(201);
  await page.goto('/note/MoveMe');
  await page.getByTitle('Edit mode', { exact: true }).click();
  await page.getByRole('textbox', { name: 'Note markdown' }).fill('draft before move');
  await page.locator('a[data-path="MoveMe"]').dragTo(page.locator('summary[data-path="Target"]'));
  await expect(page.locator('[data-note-path="Target/MoveMe"]')).toBeVisible();
  expect((await (await request.get('/api/note/Target%2FMoveMe')).json()).content).toBe('draft before move');
  expect((await request.get('/api/note/MoveMe')).status()).toBe(404);
});

test('activity refreshes when a log changes', async ({ page, request }) => {
  await createNote(request, 'log', '## 2026-10-19\n\n* **Update**: first activity\n');
  await page.goto('/');
  await page.getByRole('button', { name: 'Activity', exact: true }).click();
  await expect(page.getByText('first activity', { exact: true })).toBeVisible();
  const note = await (await request.get('/api/note/log')).json();
  expect((await request.post('/api/note/log', { headers: { 'If-Match': note.revision }, data: '## 2026-10-19\n\n* **Update**: second activity\n' })).ok()).toBeTruthy();
  await expect(page.getByText('second activity', { exact: true })).toBeVisible();
});

test('mobile navigation remains reachable', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.goto('/note/B');
  await expect(page.getByRole('button', { name: 'Open navigation', exact: true }).locator('svg')).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  await expect(page.locator('.sidebar')).not.toHaveClass(/collapsed/);
  await page.locator('a[data-path="A"]').click();
  await expect(page.getByRole('button', { name: 'Open navigation', exact: true })).toBeVisible();
});

test('graph tolerates no links, theme changes and repeated route mounts', async ({ page }) => {
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/api/graph', route => route.fulfill({ json: { nodes: [{ id: 'B', title: 'B', type: 'Note' }], links: null } }));
  for (let i = 0; i < 2; i++) {
    await page.goto('/graph');
    await expect(page.locator('.graph-info').first()).toHaveText('1 notes · 0 links');
    await page.getByTitle('Toggle theme', { exact: true }).click();
    await page.getByRole('button', { name: 'Toggle always-visible node labels' }).click();
    await page.locator('a[data-path="B"]').click();
    await expect(page.locator('[data-note-path="B"]')).toBeVisible();
  }
  expect(errors).toEqual([]);
});
