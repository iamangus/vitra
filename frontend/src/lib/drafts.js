import { notes } from './api.js';

const drafts = new Map();

export function createDraft(path, note, save = notes.save, storage = globalThis.sessionStorage) {
  const key = `vitra:draft:${path}`;
  let recovered;
  try { recovered = JSON.parse(storage?.getItem(key) || 'null'); } catch {}
  const state = {
    path,
    content: recovered?.content ?? note.content,
    revision: recovered?.revision ?? note.revision,
    savedContent: recovered ? null : note.content,
    status: recovered ? 'Recovered unsaved draft' : '',
    error: null,
  };
  let timer;
  let pending;
  const listeners = new Set();
  function notify() { for (const fn of listeners) fn(state); }
  function persist() {
    try {
      if (state.content === state.savedContent) storage?.removeItem(key);
      else storage?.setItem(key, JSON.stringify({ content: state.content, revision: state.revision }));
    } catch { state.status = 'Draft storage unavailable; keep this tab open'; }
  }
  async function flush() {
    clearTimeout(timer);
    if (pending) return pending;
    pending = (async () => {
      while (state.content !== state.savedContent) {
        const content = state.content;
        state.status = 'Saving…';
        notify();
        try {
          const result = await save(path, content, state.revision);
          state.revision = result.revision;
          state.savedContent = content;
          state.error = null;
          persist();
        } catch (error) {
          state.error = error;
          state.status = error.status === 409 ? 'Conflict — draft retained' : 'Save failed — draft retained';
          persist();
          notify();
          return;
        }
      }
      state.status = 'Saved';
      notify();
    })();
    try { await pending; } finally { pending = null; }
  }
  return {
    state,
    subscribe(fn) { listeners.add(fn); fn(state); return () => listeners.delete(fn); },
    edit(content) {
      state.content = content;
      state.status = 'Unsaved';
      persist();
      notify();
      clearTimeout(timer);
      timer = setTimeout(flush, 500);
    },
    flush,
    cancel() { clearTimeout(timer); },
    dirty: () => state.content !== state.savedContent,
  };
}

export function getDraft(path, note) {
  let draft = drafts.get(path);
  if (!draft || (!draft.dirty() && draft.state.revision !== note.revision)) {
    draft = createDraft(path, note);
    drafts.set(path, draft);
  }
  return draft;
}

export function hasUnsavedDrafts() {
  return [...drafts.values()].some(draft => draft.dirty());
}

export async function flushAffectedDrafts(path) {
  const id = path;
  for (const [key, draft] of drafts) {
    if (key !== id && !key.startsWith(id + '/')) continue;
    await draft.flush();
    if (draft.dirty()) throw new Error('Resolve the unsaved or conflicting draft before moving or deleting this note.');
  }
}

export function discardDraft(path) {
  drafts.get(path)?.cancel();
  drafts.delete(path);
  try { sessionStorage.removeItem(`vitra:draft:${path}`); } catch {}
}
