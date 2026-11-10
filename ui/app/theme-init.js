// Resolve before paint; preference stays compatible with previous releases.
(() => {
  let preference = 'auto';
  try { preference = localStorage.getItem('nm-theme') || 'auto'; } catch {}
  if (!['auto','light','dark'].includes(preference)) preference = 'auto';
  document.documentElement.dataset.preference = preference;
  document.documentElement.dataset.theme = preference === 'auto' ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : preference;
})();
