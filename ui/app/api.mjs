/** The existing localhost API contract. Mutations are never retried. */
export function createClient(fetcher = globalThis.fetch.bind(globalThis)) {
  const reads = new Map();
  return function request(path, {body, signal, fresh=false} = {}) {
    const key = path.replace(/^\/?api\//, '').replace(/^\//, '');
    const mutation = body !== undefined;
    if (!mutation && !signal && !fresh && reads.has(key)) return reads.get(key);
    const operation = (async () => {
      const response = await fetcher(`/api/${key}`, {
        method: mutation ? 'POST' : 'GET',
        headers: {'X-NetMerge': '1', ...(mutation ? {'Content-Type': 'application/json'} : {})},
        ...(mutation ? {body: JSON.stringify(body)} : {}),
        ...(signal ? {signal} : {})
      });
      const raw = await response.text();
      let value;
      try { value = raw ? JSON.parse(raw) : {}; } catch { value = null; }
      if (!response.ok) throw new Error(value?.error || `Request failed (${response.status})`);
      if (value === null) throw new Error('The application returned an unreadable response.');
      return value;
    })();
    if (!mutation && !signal && !fresh) {
      reads.set(key, operation);
      operation.then(() => reads.delete(key), () => reads.delete(key));
    }
    return operation;
  };
}
