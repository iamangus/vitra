const API_BASE = '/api';

async function api(endpoint, options = {}) {
  const url = `${API_BASE}${endpoint}`;
  const res = await fetch(url, {
    ...options,
    headers: {
      ...(options.headers || {}),
    },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
  if (res.status === 204) return null;
  const contentType = res.headers.get('content-type');
  if (contentType && contentType.includes('application/json')) {
    return res.json();
  }
  return res.text();
}

export const files = {
  list: (activePath = '') => api(`/files?active=${encodeURIComponent(activePath)}`),
};

export const notes = {
  get: (path) => api(`/note/${encodeURIComponent(path)}`),
  save: (path, content) => api(`/note/${encodeURIComponent(path)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'text/plain' },
    body: content,
  }),
  create: (path) => api('/notes', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  }),
  preview: (path, content) => api(`/preview/${encodeURIComponent(path)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'text/plain' },
    body: content,
  }),
};

export const folders = {
  create: (path) => api('/folders', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  }),
};

export const fileOps = {
  rename: (oldPath, newPath) => api('/rename', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ old: oldPath, new: newPath }),
  }),
  delete: (path) => api(`/delete?path=${encodeURIComponent(path)}`, {
    method: 'DELETE',
  }),
};

export const search = {
  query: (q) => api(`/search?q=${encodeURIComponent(q)}`),
};

export const backlinks = {
  get: (path) => api(`/backlinks/${encodeURIComponent(path)}`),
};
