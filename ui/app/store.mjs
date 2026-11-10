const clone = value => structuredClone(value);
const same = (a,b) => JSON.stringify(a) === JSON.stringify(b);
const keyed = {links:'name', appRules:'process'};

/** Record only fields the user edited, against their confirmed snapshot. */
export function captureChanges(base = {}, draft = {}) {
  const changes = [];
  for (const field of new Set([...Object.keys(base), ...Object.keys(draft)])) {
    if (same(base[field], draft[field])) continue;
    const key = keyed[field];
    if (!key) { changes.push({field, value:clone(draft[field]), remove:!(field in draft)}); continue; }
    const before = new Map((base[field] || []).map(item => [item[key], item]));
    const after = new Map((draft[field] || []).map(item => [item[key], item]));
    for (const [id,item] of before) {
      if (!after.has(id)) changes.push({field,key,id,remove:true});
      else {
        const patch = captureChanges(item, after.get(id));
        if (patch.length) changes.push({field,key,id,patch});
      }
    }
    for (const [id,item] of after) if (!before.has(id)) changes.push({field,key,id,add:clone(item)});
  }
  return changes;
}

/** Merge a draft into fresh server state without overwriting untouched fields. */
export function applyChanges(latest, changes) {
  const next = clone(latest);
  for (const change of changes) {
    const {field,key,id} = change;
    if (!key) { if (change.remove) delete next[field]; else next[field] = clone(change.value); continue; }
    const items = next[field] || [];
    const at = items.findIndex(item => item[key] === id);
    if (change.remove) { if (at >= 0) items.splice(at,1); }
    else if (change.add) { if (at < 0) items.push(clone(change.add)); else items[at] = {...items[at],...clone(change.add)}; }
    else if (at >= 0) items[at] = applyChanges(items[at],change.patch);
    next[field] = items;
  }
  return next;
}

export function createSettingsWriter({read,write}) {
  let tail = Promise.resolve();
  function saveChanges(intent) {
      const changes = clone(intent);
      const operation = tail.then(async () => {
        const current = await read();
        if (!changes.length) return current;
        const next = applyChanges(current,changes);
        await write(next);
        return next;
      });
      tail = operation.catch(() => {});
      return operation;
  }
  function transaction(task){const operation=tail.then(task);tail=operation.catch(()=>{});return operation;}
  return {save:(base,draft)=>saveChanges(captureChanges(base,draft)),saveChanges,transaction};
}
