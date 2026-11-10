export const toMbps = value => typeof value === 'number' && Number.isFinite(value) ? value * 8 / 1e6 : null;
export function contributingLinks(snap = {}) {
  return snap.running ? (snap.links || []).filter(link => link.enabled && link.present && link.healthy === true && !link.standby && !link.slow && !link.portal && !link.quotaOut) : [];
}
export function normalizeState(value) {
  const state = {...value, snap:{...value.snap}, settings:{...value.settings}, test:{...value.test}};
  for (const key of ['links','apps','history']) state.snap[key] = state.snap[key] || [];
  for (const key of ['links','appRules','bypassIPs','stableDomains']) state.settings[key] = state.settings[key] || [];
  state.adapters ||= [];
  state.test.results ||= [];
  return state;
}
