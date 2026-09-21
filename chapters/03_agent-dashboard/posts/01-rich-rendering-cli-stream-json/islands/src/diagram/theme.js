import { useState, useLayoutEffect } from 'preact/hooks';

// The island has to match whatever the surrounding page is doing, and Ghost
// themes switch dark mode in different ways (html.dark-mode, data-theme,
// body classes, or just prefers-color-scheme). Rather than guess the
// mechanism, look at the effective background colour behind the mount point
// and pick light/dark from its luminance; re-check whenever the page's theme
// hooks or the OS preference change.

const DARK_QUERY = '(prefers-color-scheme: dark)';

function parseRgb(color) {
  const m = /rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(color || '');
  if (!m) return null;
  return { r: +m[1], g: +m[2], b: +m[3], a: m[4] == null ? 1 : +m[4] };
}

export function detectTheme(fromEl) {
  let el = fromEl;
  while (el) {
    const rgb = parseRgb(getComputedStyle(el).backgroundColor);
    if (rgb && rgb.a > 0) {
      const lum = 0.2126 * rgb.r + 0.7152 * rgb.g + 0.0722 * rgb.b;
      return lum < 128 ? 'dark' : 'light';
    }
    el = el.parentElement;
  }
  return window.matchMedia && window.matchMedia(DARK_QUERY).matches ? 'dark' : 'light';
}

export function usePageTheme(ref) {
  const [theme, setTheme] = useState('light');

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return undefined;
    const update = () => setTheme(detectTheme(el.parentElement || el));
    update();

    const observer = new MutationObserver(update);
    const opts = { attributes: true, attributeFilter: ['class', 'style', 'data-theme', 'data-color-scheme'] };
    observer.observe(document.documentElement, opts);
    if (document.body) observer.observe(document.body, opts);

    const mq = window.matchMedia ? window.matchMedia(DARK_QUERY) : null;
    if (mq) mq.addEventListener('change', update);

    return () => {
      observer.disconnect();
      if (mq) mq.removeEventListener('change', update);
    };
  }, [ref]);

  return theme;
}
