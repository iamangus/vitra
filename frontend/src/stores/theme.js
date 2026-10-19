import { writable } from 'svelte/store';

function createThemeStore() {
  let mode = 'system';
  try { const saved = localStorage.getItem('theme'); if (['system', 'light', 'dark'].includes(saved)) mode = saved; } catch {}
  const preference = window.matchMedia('(prefers-color-scheme: dark)');
  const { subscribe, set } = writable('light');
  function apply() {
    const dark = mode === 'dark' || (mode === 'system' && preference.matches);
    document.documentElement.classList.toggle('dark', dark);
    document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
    set(dark ? 'dark' : 'light');
  }
  preference.addEventListener('change', () => { if (mode === 'system') apply(); });
  apply();
  return {
    subscribe,
    set(value) {
      if (!['system', 'light', 'dark'].includes(value)) return;
      mode = value;
      try { localStorage.setItem('theme', value); } catch {}
      apply();
    },
    toggle() { this.set(mode === 'system' ? 'dark' : mode === 'dark' ? 'light' : 'system'); },
  };
}

export const theme = createThemeStore();
