const timer = NM.schedule('state', () => poll(), 1000);
addEventListener('resize', () => { if (S && local) render(); });
addEventListener('hashchange', () => go(location.hash, true));
document.addEventListener('visibilitychange', () => { if (!document.hidden) poll(); });
go(location.hash || '#/overview', true);
poll(true);
