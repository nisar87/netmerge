// NetMerge browser extension: the parts that do not touch the browser, so they can be tested with plain Node.
var NMCore = (function () {
  var BIG = ['zip', 'rar', '7z', 'iso', 'exe', 'msi', 'dmg', 'pkg', 'mkv', 'mp4', 'avi', 'mov', 'wmv', 'flv', 'webm', 'flac',
    'tar', 'gz', 'tgz', 'bz2', 'xz', 'apk', 'xapk', 'deb', 'rpm', 'img', 'bin', 'cab', 'appx', 'msix', 'vhd', 'vhdx', 'wim', 'esd', 'm4v', 'ts'];

  function hostOf(u) { try { return new URL(u).hostname.toLowerCase().replace(/^\[|\]$/g, ''); } catch (e) { return ''; } }

  // Addresses that are inside the user's own network: NetMerge refuses them, so the browser should keep them.
  function isLocalHost(h) {
    if (!h) return true;
    if (h === 'localhost' || h.endsWith('.localhost') || h.endsWith('.local') || h.endsWith('.lan') || h.endsWith('.home') || h.endsWith('.internal')) return true;
    if (h.indexOf('.') < 0 && h.indexOf(':') < 0) return true; // a name without a dot, such as "nas"
    var m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)$/.exec(h);
    if (m) {
      var a = +m[1], b = +m[2];
      return a === 10 || a === 127 || a === 0 || (a === 169 && b === 254) || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 100 && b >= 64 && b <= 127);
    }
    if (h.indexOf(':') >= 0) { // IPv6
      return h === '::1' || h === '::' || /^f[cd]/.test(h) || /^fe[89ab]/.test(h) || /^::ffff:/.test(h);
    }
    return false;
  }

  function baseName(p) { return String(p || '').split(/[\\/]/).pop(); }

  function extOf(name) {
    var n = baseName(name).split('?')[0];
    var i = n.lastIndexOf('.');
    return i < 0 ? '' : n.slice(i + 1).toLowerCase();
  }

  function pathOf(u) { try { return new URL(u).pathname; } catch (e) { return ''; } }

  function ignored(host, list) {
    for (var i = 0; i < (list || []).length; i++) {
      var s = String(list[i]).trim().toLowerCase();
      if (!s) continue;
      if (host === s || host.endsWith('.' + s)) return true;
    }
    return false;
  }

  function parseIgnore(text) {
    return String(text || '').split(/[\s,;]+/).map(function (s) { return s.replace(/^https?:\/\//, '').replace(/\/.*$/, '').trim().toLowerCase(); }).filter(Boolean).slice(0, 200);
  }

  // cfg: {paired, enabled (from NetMerge), localOn, minMB, ignore:[...]}; item: a browser download item.
  function shouldCatch(item, cfg) {
    var no = function (why) { return { catch: false, why: why }; };
    if (!cfg || !cfg.paired) return no('not paired');
    if (cfg.enabled === false || cfg.localOn === false) return no('off');
    if (!item || !item.url) return no('no link');
    if (!/^https?:/i.test(item.url)) return no('not a web link');
    if (item.incognito) return no('private window');
    if (item.byExtensionId) return no('another extension');
    var host = hostOf(item.url);
    if (isLocalHost(host)) return no('local address');
    if (ignored(host, cfg.ignore)) return no('ignored site');
    var minB = Math.max(0, +cfg.minMB || 0) * 1048576;
    var size = item.fileSize > 0 ? item.fileSize : (item.totalBytes > 0 ? item.totalBytes : -1);
    if (size > 0) return size >= minB ? { catch: true, why: 'big enough' } : no('small file');
    if (minB === 0) return { catch: true, why: 'everything' };
    var ext = extOf(item.filename) || extOf(pathOf(item.url));
    if (ext && BIG.indexOf(ext) >= 0) return { catch: true, why: 'size unknown, big file type' };
    return no('size unknown');
  }

  function cookieHeader(cookies) {
    return (cookies || []).filter(function (c) { return c && c.name; }).map(function (c) { return c.name + '=' + c.value; }).join('; ');
  }

  function cleanName(p) {
    var n = baseName(p);
    return /^[^\x00-\x1f<>:"|?*]{1,200}$/.test(n) ? n : '';
  }

  return { shouldCatch: shouldCatch, isLocalHost: isLocalHost, hostOf: hostOf, parseIgnore: parseIgnore, cookieHeader: cookieHeader, cleanName: cleanName, extOf: extOf, BIG: BIG };
})();
if (typeof module !== 'undefined') module.exports = NMCore;
