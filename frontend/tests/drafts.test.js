import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createDraft } from '../src/lib/drafts.js';
import { notes } from '../src/lib/api.js';

function storage() {
  const data = new Map();
  return { getItem: key => data.get(key), setItem: (key, value) => data.set(key, value), removeItem: key => data.delete(key) };
}

test('saves remain bound to their own note after navigation', async () => {
  const writes = [];
  const save = async (path, content, revision) => { writes.push({ path, content, revision }); return { revision: 'next' }; };
  const a = createDraft('A', { content: 'A original', revision: 'a1' }, save, storage());
  a.edit('A draft');
  const b = createDraft('B', { content: 'B original', revision: 'b1' }, save, storage());
  await a.flush();
  assert.deepEqual(writes, [{ path: 'A', content: 'A draft', revision: 'a1' }]);
  assert.equal(b.state.content, 'B original');
});

test('serializes in-flight saves using the latest revision', async () => {
  let resolve;
  const writes = [];
  const save = async (path, content, revision) => {
    writes.push({ content, revision });
    if (writes.length === 1) await new Promise(done => { resolve = done; });
    return { revision: `r${writes.length}` };
  };
  const draft = createDraft('A', { content: '', revision: 'r0' }, save, storage());
  draft.edit('first');
  const pending = draft.flush();
  draft.edit('second');
  resolve();
  await pending;
  assert.deepEqual(writes, [{ content: 'first', revision: 'r0' }, { content: 'second', revision: 'r1' }]);
  assert.equal(draft.dirty(), false);
});

test('conflicting drafts survive failure and reload without changing base revision', async () => {
  const memory = storage();
  const draft = createDraft('A', { content: 'original', revision: 'r0' }, async () => { throw Object.assign(new Error('Conflict'), { status: 409 }); }, memory);
  draft.edit('important draft');
  await draft.flush();
  const recovered = createDraft('A', { content: 'external update', revision: 'r1' }, async () => ({}), memory);
  assert.equal(recovered.state.content, 'important draft');
  assert.equal(recovered.state.revision, 'r0');
  assert.equal(draft.dirty(), true);
  assert.equal(draft.state.status, 'Conflict — draft retained');
});

test('API errors preserve HTTP status for missing-note UI', async () => {
  const original = globalThis.fetch;
  globalThis.fetch = async () => new Response('Not Found', { status: 404 });
  try { await assert.rejects(notes.get('missing'), error => error.status === 404); }
  finally { globalThis.fetch = original; }
});
