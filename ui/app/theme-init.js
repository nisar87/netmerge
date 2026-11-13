// Resolve before paint; preference stays compatible with previous releases.
(() => {
  let preference = 'auto', motion = 'normal';
  try { preference = localStorage.getItem('nm-theme') || 'auto'; motion = localStorage.getItem('nm-motion') || 'normal'; } catch {}
  if (!['auto','light','dark'].includes(preference)) preference = 'auto';
  if (!['normal','reduced','off'].includes(motion)) motion = 'normal';
  const root = document.documentElement;
  root.dataset.preference = preference;
  root.dataset.theme = preference === 'auto' ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : preference;
  // animations: the user's choice, but "Normal" gives way to Windows' "Animation effects: off"
  root.dataset.motionPref = motion;
  root.dataset.motion = motion === 'normal' && matchMedia('(prefers-reduced-motion: reduce)').matches ? 'reduced' : motion;
})();
