import { writable } from 'svelte/store';

function createThemeStore() {
  const saved = localStorage.getItem('theme');
  const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const initial = saved || 'system';
  const isDark = initial === 'dark' || (initial === 'system' && systemDark);

  if (isDark) {
    document.documentElement.classList.add('dark');
  }
  document.documentElement.setAttribute('data-theme', initial);

  const { subscribe, set } = writable(initial);

  return {
    subscribe,
    set: (value) => {
      localStorage.setItem('theme', value);
      document.documentElement.setAttribute('data-theme', value);
      const dark = value === 'dark' || (value === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
      if (dark) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
      set(value);
    },
    toggle: () => {
      const current = localStorage.getItem('theme') || 'system';
      let next;
      if (current === 'system') next = 'dark';
      else if (current === 'dark') next = 'light';
      else next = 'system';
      theme.set(next);
    }
  };
}

export const theme = createThemeStore();
