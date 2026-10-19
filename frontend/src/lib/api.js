const API_BASE = "/api";

export class APIError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

async function api(endpoint, options = undefined) {
  options ??= {};
  const url = `${API_BASE}${endpoint}`;
  const res = await fetch(url, {
    ...options,
    headers: {
      ...(options.headers || {}),
    },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new APIError(text || `HTTP ${res.status}`, res.status);
  }
  if (res.status === 204) return null;
  const contentType = res.headers.get("content-type");
  if (contentType && contentType.includes("application/json")) {
    return res.json();
  }
  return res.text();
}

export const files = {
  list: (activePath = "") =>
    api(`/files?active=${encodeURIComponent(activePath)}`),
  download: (path) => {
    window.location.href = `/api/download?path=${encodeURIComponent(path)}`;
  },
};

export const notes = {
  get: (path, signal = undefined) =>
    api(`/note/${encodeURIComponent(path)}`, { signal }),
  save: (path, content, revision) =>
    api(`/note/${encodeURIComponent(path)}`, {
      method: "POST",
      headers: {
        "Content-Type": "text/plain",
        ...(revision ? { "If-Match": revision } : {}),
      },
      body: content,
    }),
  create: (path) =>
    api("/notes", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    }),
  preview: (path, content, signal = undefined) =>
    api(`/preview/${encodeURIComponent(path)}`, {
      method: "POST",
      headers: { "Content-Type": "text/plain" },
      body: content,
      signal,
    }),
};

export const folders = {
  create: (path) =>
    api("/folders", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    }),
};

export const fileOps = {
  rename: (oldPath, newPath) =>
    api("/rename", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ old: oldPath, new: newPath }),
    }),
  delete: (path) =>
    api(`/delete?path=${encodeURIComponent(path)}`, {
      method: "DELETE",
    }),
};

export const search = {
  query: (q, signal = undefined) =>
    api(`/search?q=${encodeURIComponent(q)}`, { signal }),
  semantic: (q, limit = 5, signal = undefined) =>
    api(`/search/semantic?q=${encodeURIComponent(q)}&limit=${limit}`, {
      signal,
    }),
};

export const backlinks = {
  get: (path) => api(`/backlinks/${encodeURIComponent(path)}`),
};

export const graph = {
  get: (signal = undefined) => api("/graph", { signal }),
};

export const concepts = {
  list: (params = {}) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== "") {
        if (Array.isArray(v)) v.forEach((x) => qs.append(k, x));
        else qs.set(k, v);
      }
    }
    return api(`/concepts?${qs.toString()}`);
  },
  closure: (path, depth = 2) =>
    api(`/concepts/closure?path=${encodeURIComponent(path)}&depth=${depth}`),
};

export const activity = {
  list: (params = {}) => {
    const qs = new URLSearchParams(params);
    return api(`/activity?${qs.toString()}`);
  },
};

export { api };
