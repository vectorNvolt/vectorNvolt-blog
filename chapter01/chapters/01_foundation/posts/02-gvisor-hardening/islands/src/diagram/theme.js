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

    // Theme switches (site toggle or OS preference) often fade the page
    // background with a CSS transition, so a check right after the change
    // still reads the old colour. Re-check once the fade has settled: on
    // transitionend of any ancestor, with a timeout fallback.
    let timer;
    const settle = () => {
      update();
      clearTimeout(timer);
      timer = setTimeout(update, 450);
    };
    const onEnd = (e) => {
      const t = e.target;
      if (t === document.documentElement || t === document.body || (t && t.contains && t.contains(el))) update();
    };
    document.addEventListener('transitionend', onEnd);

    const observer = new MutationObserver(settle);
    const opts = { attributes: true, attributeFilter: ['class', 'style', 'data-theme', 'data-color-scheme'] };
    observer.observe(document.documentElement, opts);
    if (document.body) observer.observe(document.body, opts);

    const mq = window.matchMedia ? window.matchMedia(DARK_QUERY) : null;
    if (mq) mq.addEventListener('change', settle);

    return () => {
      clearTimeout(timer);
      document.removeEventListener('transitionend', onEnd);
      observer.disconnect();
      if (mq) mq.removeEventListener('change', settle);
    };
  }, [ref]);

  return theme;
}
