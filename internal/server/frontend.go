package server

// frontendHTML is the complete single-page application served at /.
// It is compiled into the binary; no external assets are required.
//
// Architecture:
//   - State machine: normal | insert | swap | search modes
//   - Pane grid: CSS grid rebuilt whenever layout_update arrives
//   - WebSocket client: sub-protocol rdw-v1, auto-reconnect, queue flush
//   - Keyboard dispatch: loaded from GET /api/v1/bindings on startup
//   - ANSI parser: 16-colour + 256-colour + true-colour → inline CSS spans
//   - Scrollback: per-pane ring buffer (MAX_LINES = 10 000), auto-scroll
//   - Search: exact + fuzzy, scoped to focused pane
var frontendHTML = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>rdw</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#1e1e1e;--bg2:#252526;--bg3:#2d2d2d;--bg4:#3c3c3c;
  --fg:#d4d4d4;--fg2:#858585;--fg3:#cccccc;
  --accent:#0078d4;--accent2:#005a9e;--accent3:#1a8cff;
  --border:#3c3c3c;--border2:#555;
  --ok:#4ec9b0;--warn:#dcdcaa;--err:#f44747;
  --sel:#264f78;--sel2:#094771;
  --font:'Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace;
  --sz:13px;--lh:1.5;
  --hdr:28px;--status:20px;--prompt-h:32px;
}
body{background:var(--bg);color:var(--fg);font:var(--sz)/var(--lh) var(--font);
  display:flex;flex-direction:column;height:100vh;overflow:hidden;user-select:none}

/* ── Header ─────────────────────────────────────────────────────────────── */
#hdr{height:var(--hdr);background:var(--bg3);border-bottom:1px solid var(--border);
  display:flex;align-items:center;padding:0 8px;gap:2px;flex-shrink:0;overflow-x:auto;
  scrollbar-width:none}
#hdr::-webkit-scrollbar{display:none}
.logo{color:var(--accent);font-weight:700;padding:0 10px 0 2px;font-size:12px;
  letter-spacing:.05em;flex-shrink:0}
.wtab{padding:2px 12px;border-radius:3px;cursor:pointer;font-size:12px;
  border:1px solid transparent;white-space:nowrap;transition:background .1s;flex-shrink:0}
.wtab:hover{background:var(--bg4)}
.wtab.active{background:var(--accent);color:#fff;border-color:var(--accent2)}
.hdr-spacer{flex:1}
#mode-badge{font-size:11px;color:var(--fg2);padding:0 8px;flex-shrink:0}
#search-bar{display:none;align-items:center;gap:6px;margin-left:8px}
#search-bar.visible{display:flex}
#search-input{background:var(--bg2);border:1px solid var(--accent);color:var(--fg);
  font:var(--sz) var(--font);padding:2px 8px;border-radius:3px;width:220px;outline:none}
#search-matches{font-size:11px;color:var(--fg2)}

/* ── Workspace ───────────────────────────────────────────────────────────── */
#ws{flex:1;display:grid;overflow:hidden;position:relative}
#ws.zoom-active>.pane:not(.zoomed){display:none!important}
#ws.zoom-active>.pane.zoomed{grid-area:1/1/-1/-1!important}

/* ── Panes ────────────────────────────────────────────────────────────────── */
.pane{display:flex;flex-direction:column;border:1px solid var(--border);
  min-width:60px;min-height:40px;position:relative;overflow:hidden}
.pane.focused>.phdr{border-bottom-color:var(--accent)}
.pane.focused{border-color:var(--accent)}
.pane.swap-target{border-color:var(--warn)!important;border-style:dashed}
.phdr{background:var(--bg2);padding:2px 8px;font-size:11px;color:var(--fg2);
  border-bottom:1px solid var(--border);flex-shrink:0;display:flex;align-items:center;gap:6px;
  cursor:default}
.phdr .pid{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.phdr .pbtn{cursor:pointer;opacity:.5;font-size:10px;padding:0 3px}
.phdr .pbtn:hover{opacity:1}
.pbody{flex:1;overflow-y:auto;padding:3px 6px;font-size:var(--sz);line-height:var(--lh);
  scrollbar-width:thin;scrollbar-color:var(--bg4) transparent;user-select:text}
.pbody::-webkit-scrollbar{width:5px}
.pbody::-webkit-scrollbar-thumb{background:var(--bg4);border-radius:2px}
.line{display:block;white-space:pre-wrap;word-break:break-all}
.line.match{background:var(--sel)}
.line.match.current{background:var(--sel2);outline:1px solid var(--accent3)}
.ts{color:var(--fg2);font-size:11px;margin-right:6px}

/* ── Gutter ───────────────────────────────────────────────────────────────── */
.gutter{background:transparent;z-index:10;transition:background .15s}
.gutter:hover,.gutter.dragging{background:var(--accent);opacity:.4}
.gutter-v{cursor:col-resize;width:4px;margin:0 -2px}
.gutter-h{cursor:row-resize;height:4px;margin:-2px 0}

/* ── Status bar ───────────────────────────────────────────────────────────── */
#status{height:var(--status);background:#007acc;color:#fff;font-size:11px;
  display:flex;align-items:center;padding:0 10px;gap:12px;flex-shrink:0}
.s-sep{opacity:.4}
#s-conn{font-weight:500}
#s-conn.err{color:#ffb3b3}
#s-conn.warn{color:#ffe082}

/* ── Prompt overlay ────────────────────────────────────────────────────────── */
#overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,.55);
  z-index:100;align-items:center;justify-content:center}
#overlay.visible{display:flex}
#prompt-box{background:var(--bg3);border:1px solid var(--border2);border-radius:5px;
  padding:16px 20px;min-width:300px;display:flex;flex-direction:column;gap:10px}
#prompt-label{font-size:12px;color:var(--fg2)}
#prompt-input{background:var(--bg);border:1px solid var(--accent);color:var(--fg);
  font:var(--sz) var(--font);padding:5px 10px;border-radius:3px;outline:none;width:100%}
#prompt-buttons{display:flex;justify-content:flex-end;gap:8px}
.pbtn-ok,.pbtn-cancel{padding:4px 14px;border:none;border-radius:3px;cursor:pointer;
  font:var(--sz) var(--font)}
.pbtn-ok{background:var(--accent);color:#fff}
.pbtn-cancel{background:var(--bg4);color:var(--fg)}

/* ── Context menu ─────────────────────────────────────────────────────────── */
#ctx-menu{display:none;position:fixed;background:var(--bg3);border:1px solid var(--border2);
  border-radius:4px;z-index:200;min-width:160px;padding:4px 0;box-shadow:0 4px 16px rgba(0,0,0,.5)}
#ctx-menu.visible{display:block}
.ctx-item{padding:5px 16px;cursor:pointer;font-size:12px}
.ctx-item:hover{background:var(--accent)}
.ctx-sep{height:1px;background:var(--border);margin:4px 0}
</style>
</head>
<body>

<!-- Header -->
<div id="hdr">
  <span class="logo">rdw</span>
  <span id="win-tabs"></span>
  <span class="hdr-spacer"></span>
  <span id="mode-badge"></span>
  <div id="search-bar">
    <input id="search-input" placeholder="search…" autocomplete="off" spellcheck="false">
    <span id="search-matches"></span>
  </div>
</div>

<!-- Workspace -->
<div id="ws"></div>

<!-- Status bar -->
<div id="status">
  <span id="s-conn" class="warn">connecting…</span>
  <span class="s-sep">|</span>
  <span id="s-focus">—</span>
  <span class="s-sep">|</span>
  <span id="s-lines">0 lines</span>
  <span class="s-sep">|</span>
  <span id="s-port">rdw-v1</span>
</div>

<!-- Prompt overlay -->
<div id="overlay">
  <div id="prompt-box">
    <div id="prompt-label">Enter value</div>
    <input id="prompt-input" autocomplete="off" spellcheck="false">
    <div id="prompt-buttons">
      <button class="pbtn-cancel" id="prompt-cancel">Cancel</button>
      <button class="pbtn-ok" id="prompt-ok">OK</button>
    </div>
  </div>
</div>

<!-- Context menu -->
<div id="ctx-menu">
  <div class="ctx-item" data-action="pane.zoom">Zoom / unzoom</div>
  <div class="ctx-item" data-action="pane.split.v">Split right</div>
  <div class="ctx-item" data-action="pane.split.h">Split below</div>
  <div class="ctx-sep"></div>
  <div class="ctx-item" data-action="pane.rename">Rename pane</div>
  <div class="ctx-item" data-action="scroll.clear">Clear scrollback</div>
  <div class="ctx-sep"></div>
  <div class="ctx-item" data-action="pane.close">Close pane</div>
</div>

<script>
'use strict';
(function(){

// ═══════════════════════════════════════════════════════════════════════════
// Constants
// ═══════════════════════════════════════════════════════════════════════════
var MAX_LINES   = 10000;
var RESIZE_STEP = 5;       // percent per keypress
var API         = '/api/v1';
var WS_PROTO    = 'rdw-v1';
var RECONNECT_MS = 2000;

// ═══════════════════════════════════════════════════════════════════════════
// State
// ═══════════════════════════════════════════════════════════════════════════
var state = {
  windows:      [],   // [{name, panes:[{target_id,split,size,...}]}]
  activeWin:    0,
  focusedPane:  null, // target_id string
  mode:         'normal',  // normal | insert | swap | search
  swapSource:   null,
  searchQuery:  '',
  searchIdx:    0,
  searchResults:[],  // [{paneEl, lineEl}]
  zoom:         null,  // target_id of zoomed pane, or null
  bindings:     {},  // action -> [key,...]
  keySeq:       [],  // pending multi-key sequence
  totalLines:   0,
  scrollbacks:  {},  // target_id -> [line strings]
};

// ═══════════════════════════════════════════════════════════════════════════
// DOM refs
// ═══════════════════════════════════════════════════════════════════════════
var ws_el      = document.getElementById('ws');
var hdr_tabs   = document.getElementById('win-tabs');
var mode_badge = document.getElementById('mode-badge');
var s_conn     = document.getElementById('s-conn');
var s_focus    = document.getElementById('s-focus');
var s_lines    = document.getElementById('s-lines');
var overlay    = document.getElementById('overlay');
var prompt_lbl = document.getElementById('prompt-label');
var prompt_inp = document.getElementById('prompt-input');
var prompt_ok  = document.getElementById('prompt-ok');
var prompt_can = document.getElementById('prompt-cancel');
var search_bar = document.getElementById('search-bar');
var search_inp = document.getElementById('search-input');
var search_mtc = document.getElementById('search-matches');
var ctx_menu   = document.getElementById('ctx-menu');

// ═══════════════════════════════════════════════════════════════════════════
// ANSI colour parser
// ═══════════════════════════════════════════════════════════════════════════
var ANSI16_FG = ['#000000','#cc0000','#00cc00','#cccc00',
                 '#0000cc','#cc00cc','#00cccc','#cccccc',
                 '#888888','#ff4444','#44ff44','#ffff44',
                 '#4444ff','#ff44ff','#44ffff','#ffffff'];
var ANSI16_BG = ANSI16_FG;

function ansiToHtml(text) {
  // Escape HTML first
  text = text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  var out = '';
  var re  = /\x1b\[([0-9;]*)m/g;
  var last = 0;
  var openSpans = 0;
  var m;
  while ((m = re.exec(text)) !== null) {
    out += text.slice(last, m.index);
    last = m.index + m[0].length;
    var codes = m[1] === '' ? ['0'] : m[1].split(';');
    var styles = parseAnsiCodes(codes);
    if (styles === null) {
      // reset
      while (openSpans-- > 0) out += '</span>';
      openSpans = 0;
    } else if (styles.length) {
      out += '<span style="' + styles.join(';') + '">';
      openSpans++;
    }
  }
  out += text.slice(last);
  while (openSpans-- > 0) out += '</span>';
  return out;
}

function parseAnsiCodes(codes) {
  var i = 0, styles = [];
  while (i < codes.length) {
    var n = parseInt(codes[i], 10) || 0;
    if (n === 0)  return null;
    if (n === 1)  styles.push('font-weight:bold');
    if (n === 3)  styles.push('font-style:italic');
    if (n === 4)  styles.push('text-decoration:underline');
    if (n === 7)  styles.push('filter:invert(1)');
    // 16-colour fg
    if (n >= 30 && n <= 37) styles.push('color:'+ANSI16_FG[n-30]);
    if (n === 39) styles.push('color:inherit');
    // bright fg
    if (n >= 90 && n <= 97) styles.push('color:'+ANSI16_FG[n-82]);
    // 16-colour bg
    if (n >= 40 && n <= 47) styles.push('background:'+ANSI16_BG[n-40]);
    if (n === 49) styles.push('background:inherit');
    // bright bg
    if (n >= 100 && n <= 107) styles.push('background:'+ANSI16_BG[n-92]);
    // 256-colour / true-colour
    if ((n === 38 || n === 48) && codes[i+1] === '5' && codes[i+2] !== undefined) {
      var idx = parseInt(codes[i+2], 10);
      var col = ansi256(idx);
      styles.push((n===38?'color:':'background:')+col);
      i += 2;
    }
    if ((n === 38 || n === 48) && codes[i+1] === '2' &&
        codes[i+2] !== undefined && codes[i+3] !== undefined && codes[i+4] !== undefined) {
      var r=codes[i+2],g=codes[i+3],b=codes[i+4];
      styles.push((n===38?'color:':'background:')+'rgb('+r+','+g+','+b+')');
      i += 4;
    }
    i++;
  }
  return styles;
}

function ansi256(n) {
  if (n < 16)  return ANSI16_FG[n];
  if (n >= 232) { var v = 8+(n-232)*10; return 'rgb('+v+','+v+','+v+')'; }
  n -= 16;
  var b=n%6, g=Math.floor(n/6)%6, r=Math.floor(n/36);
  var c = function(x){ return x?55+x*40:0; };
  return 'rgb('+c(r)+','+c(g)+','+c(b)+')';
}

// ═══════════════════════════════════════════════════════════════════════════
// API helpers
// ═══════════════════════════════════════════════════════════════════════════
function api(method, path, body) {
  var opts = { method: method, headers: {} };
  if (body !== undefined && body !== null) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  return fetch(API + path, opts).then(function(r) {
    if (!r.ok) return r.json().then(function(e){ throw new Error(e.error || r.status); });
    var ct = r.headers.get('content-type') || '';
    return ct.includes('json') ? r.json() : null;
  });
}
var get  = function(p)    { return api('GET', p, null); };
var post = function(p, b) { return api('POST', p, b); };
var put  = function(p, b) { return api('PUT', p, b); };
var del  = function(p)    { return api('DELETE', p, null); };
var patch= function(p, b) { return api('PATCH', p, b); };

// ═══════════════════════════════════════════════════════════════════════════
// Prompt helper (returns Promise<string|null>)
// ═══════════════════════════════════════════════════════════════════════════
function prompt(label, initial) {
  return new Promise(function(resolve) {
    prompt_lbl.textContent = label || 'Enter value';
    prompt_inp.value = initial || '';
    overlay.classList.add('visible');
    prompt_inp.focus();
    prompt_inp.select();
    function finish(val) {
      overlay.classList.remove('visible');
      prompt_ok.onclick = null;
      prompt_can.onclick = null;
      prompt_inp.onkeydown = null;
      resolve(val);
    }
    prompt_ok.onclick  = function(){ finish(prompt_inp.value.trim()); };
    prompt_can.onclick = function(){ finish(null); };
    prompt_inp.onkeydown = function(e) {
      if (e.key === 'Enter')  finish(prompt_inp.value.trim());
      if (e.key === 'Escape') finish(null);
      e.stopPropagation();
    };
  });
}

// ═══════════════════════════════════════════════════════════════════════════
// Layout rendering
// ═══════════════════════════════════════════════════════════════════════════

// Build a CSS grid layout from the pane list.
// We use a simple binary-split model: each pane with split:'v' opens a new
// column; split:'h' opens a new row within the current column group.
function renderLayout(windows, activeIdx) {
  hdr_tabs.innerHTML = '';
  windows.forEach(function(w, i) {
    var tab = document.createElement('span');
    tab.className = 'wtab' + (i === activeIdx ? ' active' : '');
    tab.textContent = w.name;
    tab.dataset.idx = i;
    tab.addEventListener('click', function() {
      post('/windows/' + encodeURIComponent(w.name) + '/focus', null)
        .catch(console.error);
    });
    hdr_tabs.appendChild(tab);
  });

  ws_el.innerHTML = '';

  if (!windows.length) return;

  var win = windows[activeIdx] || windows[0];
  if (!win || !win.panes || !win.panes.length) return;

  buildPaneGrid(ws_el, win.panes);

  // Restore zoom if active.
  if (state.zoom) {
    var zp = ws_el.querySelector('[data-id="'+CSS.escape(state.zoom)+'"]');
    if (zp) {
      zp.classList.add('zoomed');
      ws_el.classList.add('zoom-active');
    } else {
      state.zoom = null;
    }
  }

  // Focus the right pane.
  focusPane(state.focusedPane || (win.panes[0] && win.panes[0].target_id));
}

// Build the pane grid using CSS grid columns/rows.
// Strategy: scan panes for v-splits (column breaks) and h-splits (row breaks).
// Each pane gets explicit grid-column / grid-row assignments.
function buildPaneGrid(container, panes) {
  // Figure out grid dimensions.
  // We walk the list tracking current column/row.
  var placements = [];
  var col = 1, row = 1, maxCol = 1, maxRow = 1;
  var colStart = [1]; // stack of column starts for each column

  panes.forEach(function(p, i) {
    if (i === 0) {
      placements.push({col:1, row:1, p:p});
      maxCol = 1; maxRow = 1;
      return;
    }
    var split = p.split || 'h';
    if (split === 'v') {
      col++;
      row = 1;
      maxCol = Math.max(maxCol, col);
    } else {
      row++;
      maxRow = Math.max(maxRow, row);
    }
    placements.push({col:col, row:row, p:p});
  });

  // Build column-template from sizes.
  var colSizes = [];
  for (var c = 1; c <= maxCol; c++) {
    var colPanes = placements.filter(function(pl){ return pl.col === c; });
    var firstSize = colPanes.length && colPanes[0].p.size ? colPanes[0].p.size : null;
    colSizes.push(cssSize(firstSize, 'col', maxCol));
  }

  var rowSizes = [];
  for (var r = 1; r <= maxRow; r++) {
    var rowPanes = placements.filter(function(pl){ return pl.row === r && pl.col === 1; });
    var rs = rowPanes.length && rowPanes[0].p.size ? rowPanes[0].p.size : null;
    rowSizes.push(cssSize(rs, 'row', maxRow));
  }

  container.style.gridTemplateColumns = colSizes.join(' ');
  container.style.gridTemplateRows    = rowSizes.join(' ');

  placements.forEach(function(pl) {
    var el = buildPane(pl.p);
    el.style.gridColumn = pl.col;
    el.style.gridRow    = pl.row;
    container.appendChild(el);

    // Vertical gutter (right of each pane except last column).
    if (pl.col < maxCol) {
      var g = document.createElement('div');
      g.className = 'gutter gutter-v';
      g.dataset.pane = pl.p.target_id;
      g.dataset.dir  = 'right';
      g.style.gridColumn = pl.col;
      g.style.gridRow    = pl.row;
      g.style.alignSelf  = 'stretch';
      attachGutterDrag(g, container, colSizes, rowSizes, 'v', pl.col-1);
      container.appendChild(g);
    }
    if (pl.row < maxRow) {
      var gh = document.createElement('div');
      gh.className = 'gutter gutter-h';
      gh.dataset.pane = pl.p.target_id;
      gh.dataset.dir  = 'down';
      gh.style.gridColumn = pl.col;
      gh.style.gridRow    = pl.row;
      gh.style.justifySelf= 'stretch';
      attachGutterDrag(gh, container, colSizes, rowSizes, 'h', pl.row-1);
      container.appendChild(gh);
    }
  });
}

function cssSize(spec, axis, total) {
  if (!spec) return '1fr';
  // N% → fr proportion
  if (spec.endsWith('%')) {
    var pct = parseFloat(spec);
    var rem = 100 - pct * (1/total * total); // rough
    return pct + 'fr';
  }
  if (spec.endsWith('px')) return spec;
  // columns: treat plain number as char width
  return spec + 'ch';
}

function buildPane(paneData) {
  var id = paneData.target_id;
  var el = document.createElement('div');
  el.className = 'pane';
  el.dataset.id = id;

  var hdr = document.createElement('div');
  hdr.className = 'phdr';
  hdr.innerHTML = '<span class="pid">' + escHtml(id) + '</span>' +
    '<span class="pbtn" data-action="pane.zoom" title="Zoom (z)">⬜</span>' +
    '<span class="pbtn" data-action="pane.close" title="Close (q)">✕</span>';
  hdr.querySelectorAll('.pbtn').forEach(function(btn) {
    btn.addEventListener('click', function(e) {
      e.stopPropagation();
      dispatch(btn.dataset.action, id);
    });
  });

  var body = document.createElement('div');
  body.className = 'pbody';
  body.dataset.id = id;

  // Restore scrollback from state.
  if (state.scrollbacks[id]) {
    state.scrollbacks[id].forEach(function(line) {
      body.appendChild(makeLine(line));
    });
  }

  el.appendChild(hdr);
  el.appendChild(body);

  // Click to focus.
  el.addEventListener('mousedown', function(e) {
    if (e.target.closest('.pbtn')) return;
    focusPane(id);
    if (state.mode === 'swap') {
      if (state.swapSource && state.swapSource !== id) {
        doSwap(state.swapSource, id);
        setMode('normal');
      }
    }
  });

  // Right-click context menu.
  el.addEventListener('contextmenu', function(e) {
    e.preventDefault();
    focusPane(id);
    showCtxMenu(e.clientX, e.clientY, id);
  });

  // Double-click header = zoom.
  hdr.addEventListener('dblclick', function() { dispatch('pane.zoom', id); });

  return el;
}

function makeLine(text) {
  var span = document.createElement('span');
  span.className = 'line';
  span.innerHTML = ansiToHtml(text);
  return span;
}

// ═══════════════════════════════════════════════════════════════════════════
// Gutter drag-to-resize
// ═══════════════════════════════════════════════════════════════════════════
function attachGutterDrag(gutter, container, colSizes, rowSizes, axis, idx) {
  gutter.addEventListener('mousedown', function(e) {
    e.preventDefault();
    gutter.classList.add('dragging');
    var startX = e.clientX, startY = e.clientY;
    var rect = container.getBoundingClientRect();
    var sizes = axis === 'v' ? colSizes.slice() : rowSizes.slice();

    function onMove(ev) {
      var delta = axis === 'v'
        ? (ev.clientX - startX) / rect.width  * 100
        : (ev.clientY - startY) / rect.height * 100;
      var newSizes = sizes.slice();
      // Convert all sizes to fr percentages for manipulation.
      var frSizes = newSizes.map(function(s) {
        if (s === '1fr') return 100 / newSizes.length;
        if (s.endsWith('fr')) return parseFloat(s);
        return 100 / newSizes.length;
      });
      frSizes[idx]   = Math.max(5, frSizes[idx]   + delta);
      frSizes[idx+1] = Math.max(5, frSizes[idx+1] - delta);
      var template = frSizes.map(function(f){ return f.toFixed(2)+'fr'; }).join(' ');
      if (axis === 'v') container.style.gridTemplateColumns = template;
      else              container.style.gridTemplateRows    = template;
    }

    function onUp() {
      gutter.classList.remove('dragging');
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
    }
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  });
}

// ═══════════════════════════════════════════════════════════════════════════
// Pane focus
// ═══════════════════════════════════════════════════════════════════════════
function focusPane(id) {
  if (!id) return;
  state.focusedPane = id;
  ws_el.querySelectorAll('.pane').forEach(function(el) {
    el.classList.toggle('focused', el.dataset.id === id);
  });
  s_focus.textContent = id;
}

function focusedPaneEl() {
  if (!state.focusedPane) return null;
  return ws_el.querySelector('.pane[data-id="'+CSS.escape(state.focusedPane)+'"]');
}

function focusedBodyEl() {
  var el = focusedPaneEl();
  return el ? el.querySelector('.pbody') : null;
}

// Focus relative to current pane (h/j/k/l).
function focusRelative(dir) {
  var panes = Array.from(ws_el.querySelectorAll('.pane'));
  if (!panes.length) return;
  var cur = focusedPaneEl();
  if (!cur) { focusPane(panes[0].dataset.id); return; }

  var curRect = cur.getBoundingClientRect();
  var cx = curRect.left + curRect.width/2;
  var cy = curRect.top  + curRect.height/2;
  var best = null, bestDist = Infinity;

  panes.forEach(function(p) {
    if (p === cur) return;
    var r = p.getBoundingClientRect();
    var px = r.left + r.width/2, py = r.top + r.height/2;
    var dx = px - cx, dy = py - cy;
    var ok = false;
    if (dir === 'left'  && dx < -10 && Math.abs(dy) < r.height) ok = true;
    if (dir === 'right' && dx >  10 && Math.abs(dy) < r.height) ok = true;
    if (dir === 'up'    && dy < -10 && Math.abs(dx) < r.width)  ok = true;
    if (dir === 'down'  && dy >  10 && Math.abs(dx) < r.width)  ok = true;
    if (ok) {
      var dist = Math.abs(dx) + Math.abs(dy);
      if (dist < bestDist) { bestDist = dist; best = p; }
    }
  });

  if (best) focusPane(best.dataset.id);
}

// ═══════════════════════════════════════════════════════════════════════════
// Line append
// ═══════════════════════════════════════════════════════════════════════════
function applyHighlightProfile(targetID, rules) {
  var el = document.getElementById('pane-' + targetID);
  if (!el) return;
  el.querySelectorAll('.rdw-line').forEach(function(line) {
    var text = line.textContent;
    rules.forEach(function(rule) {
      try {
        var re = new RegExp(rule.pattern, 'g');
        text = text.replace(re, function(m) {
          return '<span class="' + rule['class'] + '">' + m + '</span>';
        });
      } catch(_) {}
    });
    line.innerHTML = text;
  });
}

function renderImage(targetID, b64data, scale) {
  var body = ws_el.querySelector('.pbody[data-id="'+CSS.escape(targetID)+'"]');
  if (!body) return;

  var wrapper = document.createElement('div');
  wrapper.className = 'rdw-image-block rdw-scale-' + scale;

  var img = document.createElement('img');
  img.src = 'data:image/png;base64,' + b64data;
  img.className = 'rdw-img';
  img.alt = '';
  img.onerror = function() {
    // Try JPEG if PNG fails; server already b64-encoded the raw bytes.
    img.onerror = null;
  };

  wrapper.appendChild(img);
  body.appendChild(wrapper);

  var atBottom = body.scrollHeight - body.scrollTop - body.clientHeight < 40;
  if (atBottom) body.scrollTop = body.scrollHeight;
}

function renderSVG(targetID, b64data, scale) {
  var body = ws_el.querySelector('.pbody[data-id="'+CSS.escape(targetID)+'"]');
  if (!body) return;

  var wrapper = document.createElement('div');
  wrapper.className = 'rdw-svg-block rdw-scale-' + scale;

  try {
    var svgText = atob(b64data);
    wrapper.innerHTML = svgText;
    // Ensure the embedded SVG is responsive.
    var svgEl = wrapper.querySelector('svg');
    if (svgEl) {
      svgEl.style.width  = scale === 'native' ? '' : '100%';
      svgEl.style.height = scale === 'fill'   ? '100%' : 'auto';
      svgEl.removeAttribute('width');
      svgEl.removeAttribute('height');
    }
  } catch(e) {
    wrapper.textContent = '[svg decode error]';
  }

  body.appendChild(wrapper);

  var atBottom = body.scrollHeight - body.scrollTop - body.clientHeight < 40;
  if (atBottom) body.scrollTop = body.scrollHeight;
}

function setPaneScale(targetID, scale) {
  var body = ws_el.querySelector('.pbody[data-id="'+CSS.escape(targetID)+'"]');
  if (!body) return;
  // Update all existing image/svg blocks in this pane.
  body.querySelectorAll('.rdw-image-block, .rdw-svg-block').forEach(function(el) {
    el.className = el.className.replace(/rdw-scale-\w+/, 'rdw-scale-' + scale);
    var img = el.querySelector('img');
    if (img) {
      img.style.width  = scale === 'native' ? '' : '100%';
      img.style.height = scale === 'fill'   ? '100%' : 'auto';
    }
    var svgEl = el.querySelector('svg');
    if (svgEl) {
      svgEl.style.width  = scale === 'native' ? '' : '100%';
      svgEl.style.height = scale === 'fill'   ? '100%' : 'auto';
    }
  });
}

function appendLine(targetID, text) {
  // Maintain scrollback in state.
  if (!state.scrollbacks[targetID]) state.scrollbacks[targetID] = [];
  var sb = state.scrollbacks[targetID];
  sb.push(text);
  if (sb.length > MAX_LINES) sb.splice(0, sb.length - MAX_LINES);

  state.totalLines++;
  s_lines.textContent = state.totalLines + ' lines';

  var body = ws_el.querySelector('.pbody[data-id="'+CSS.escape(targetID)+'"]');
  if (!body) return;

  var lineEl = makeLine(text);

  // Trim DOM to MAX_LINES.
  while (body.children.length >= MAX_LINES) {
    body.removeChild(body.firstChild);
  }

  body.appendChild(lineEl);

  // Auto-scroll if within 2 line-heights of bottom.
  var atBottom = body.scrollHeight - body.scrollTop - body.clientHeight < 40;
  if (atBottom) body.scrollTop = body.scrollHeight;

  // Re-run search highlight if active.
  if (state.mode === 'search' && state.searchQuery) {
    highlightLine(lineEl, state.searchQuery);
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Actions
// ═══════════════════════════════════════════════════════════════════════════
function dispatch(action, paneID) {
  var id = paneID || state.focusedPane;
  switch(action) {
    // --- Window ---
    case 'window.next':   cycleWindow(1);  break;
    case 'window.prev':   cycleWindow(-1); break;
    case 'window.first':  switchWindow(0); break;
    case 'window.last':   switchWindow(state.windows.length-1); break;
    case 'window.new':
      prompt('New window name').then(function(name) {
        if (name) post('/windows', {name:name}).then(reloadSession).catch(alert);
      });
      break;
    case 'window.close':
      post('/windows/' + encodeURIComponent(currentWindowName()) + '/focus', null)
        .then(function(){ return del('/windows/'+encodeURIComponent(currentWindowName())); })
        .then(reloadSession).catch(alert);
      break;
    case 'window.rename':
      var oldName = currentWindowName();
      prompt('Rename window', oldName).then(function(name) {
        if (name && name !== oldName)
          patch('/windows/'+encodeURIComponent(oldName), {name:name})
            .then(reloadSession).catch(alert);
      });
      break;

    // --- Pane focus ---
    case 'pane.focus.left':  focusRelative('left');  break;
    case 'pane.focus.right': focusRelative('right'); break;
    case 'pane.focus.up':    focusRelative('up');    break;
    case 'pane.focus.down':  focusRelative('down');  break;

    // --- Pane split ---
    case 'pane.split.v':
    case 'pane.split.h':
      if (!id) break;
      var dir = action === 'pane.split.v' ? 'v' : 'h';
      prompt('New pane target ID').then(function(newID) {
        if (!newID) return;
        post('/panes/'+encodeURIComponent(id)+'/split', {direction:dir, new_id:newID})
          .then(reloadSession).catch(alert);
      });
      break;

    // --- Pane resize ---
    case 'pane.resize.left':  nudgeResize(id,'left');  break;
    case 'pane.resize.right': nudgeResize(id,'right'); break;
    case 'pane.resize.up':    nudgeResize(id,'up');    break;
    case 'pane.resize.down':  nudgeResize(id,'down');  break;

    // --- Pane lifecycle ---
    case 'pane.close':
      if (!id) break;
      del('/panes/'+encodeURIComponent(id)).then(reloadSession).catch(alert);
      break;
    case 'pane.zoom':
      if (!id) break;
      if (state.zoom === id) {
        state.zoom = null;
        ws_el.classList.remove('zoom-active');
        ws_el.querySelectorAll('.pane').forEach(function(p){ p.classList.remove('zoomed'); });
      } else {
        state.zoom = id;
        ws_el.classList.add('zoom-active');
        ws_el.querySelectorAll('.pane').forEach(function(p){
          p.classList.toggle('zoomed', p.dataset.id === id);
        });
      }
      post('/panes/'+encodeURIComponent(id)+'/zoom', null).catch(console.error);
      break;
    case 'pane.rename':
      if (!id) break;
      prompt('Rename pane target ID', id).then(function(newID) {
        if (!newID || newID === id) return;
        // Swap pane in layout: split new, close old.
        // For now just alert that rename updates the ID in the session.
        // Full implementation requires a dedicated PATCH /panes/:id endpoint.
        alert('Rename not yet fully wired (requires Phase 7 PATCH endpoint). Use CLI: rdw pane split');
      });
      break;
    case 'pane.swap':
      if (!id) break;
      if (state.mode === 'swap') {
        setMode('normal');
      } else {
        state.swapSource = id;
        setMode('swap');
      }
      break;

    // --- Scroll ---
    case 'scroll.up': {
      var b = focusedBodyEl(); if (b) b.scrollTop -= b.clientHeight * 0.45; break;
    }
    case 'scroll.down': {
      var b2 = focusedBodyEl(); if (b2) b2.scrollTop += b2.clientHeight * 0.45; break;
    }
    case 'scroll.top': {
      var b3 = focusedBodyEl(); if (b3) b3.scrollTop = 0; break;
    }
    case 'scroll.bottom': {
      var b4 = focusedBodyEl(); if (b4) b4.scrollTop = b4.scrollHeight; break;
    }
    case 'scroll.clear': {
      if (!id) break;
      var b5 = ws_el.querySelector('.pbody[data-id="'+CSS.escape(id)+'"]');
      if (b5) b5.innerHTML = '';
      if (state.scrollbacks[id]) state.scrollbacks[id] = [];
      break;
    }

    // --- Search ---
    case 'search.open':
      openSearch();
      break;
    case 'search.next':
      navigateSearch(1);
      break;
    case 'search.prev':
      navigateSearch(-1);
      break;

    // --- Layout ---
    case 'layout.save':
      prompt('Save layout as', '').then(function(name) {
        if (!name) return;
        post('/layouts', {name:name}).catch(alert);
      });
      break;
    case 'layout.reload':
      reloadSession();
      break;

    // --- Mode ---
    case 'mode.escape':
      setMode('normal');
      closeSearch();
      closeCtxMenu();
      break;
  }
}

function currentWindowName() {
  var win = state.windows[state.activeWin];
  return win ? win.name : '';
}

function cycleWindow(delta) {
  if (!state.windows.length) return;
  var idx = (state.activeWin + delta + state.windows.length) % state.windows.length;
  switchWindow(idx);
}

function switchWindow(idx) {
  var win = state.windows[idx];
  if (!win) return;
  post('/windows/'+encodeURIComponent(win.name)+'/focus', null).catch(console.error);
}

function nudgeResize(id, dir) {
  if (!id) return;
  post('/panes/'+encodeURIComponent(id)+'/resize',
    {direction: dir, size: RESIZE_STEP+'%'}).catch(console.error);
}

function doSwap(a, b) {
  // Optimistic UI: swap their DOM positions.
  var elA = ws_el.querySelector('.pane[data-id="'+CSS.escape(a)+'"]');
  var elB = ws_el.querySelector('.pane[data-id="'+CSS.escape(b)+'"]');
  if (elA && elB) {
    var colA = elA.style.gridColumn, rowA = elA.style.gridRow;
    elA.style.gridColumn = elB.style.gridColumn;
    elA.style.gridRow    = elB.style.gridRow;
    elB.style.gridColumn = colA;
    elB.style.gridRow    = rowA;
    elA.dataset.id = b; elB.dataset.id = a;
    elA.querySelector('.pid').textContent = b;
    elB.querySelector('.pid').textContent = a;
  }
  // Tell server (Phase 7 endpoint).
  post('/panes/'+encodeURIComponent(a)+'/swap', {target: b}).catch(console.error);
}

// ═══════════════════════════════════════════════════════════════════════════
// Mode management
// ═══════════════════════════════════════════════════════════════════════════
var MODE_LABELS = {
  normal: '', insert: 'INSERT', swap: 'SWAP — pick target', search: 'SEARCH'
};

function setMode(m) {
  state.mode = m;
  if (m !== 'swap') state.swapSource = null;
  ws_el.querySelectorAll('.pane').forEach(function(p){
    p.classList.remove('swap-target');
  });
  if (m === 'swap') {
    ws_el.querySelectorAll('.pane:not([data-id="'+CSS.escape(state.swapSource)+'"])').forEach(function(p){
      p.classList.add('swap-target');
    });
  }
  mode_badge.textContent = MODE_LABELS[m] || '';
}

// ═══════════════════════════════════════════════════════════════════════════
// Search
// ═══════════════════════════════════════════════════════════════════════════
function openSearch() {
  setMode('search');
  search_bar.classList.add('visible');
  search_inp.value = state.searchQuery || '';
  search_inp.focus();
  search_inp.select();
}

function closeSearch() {
  setMode('normal');
  search_bar.classList.remove('visible');
  clearSearchHighlights();
}

function runSearch(q) {
  state.searchQuery = q;
  clearSearchHighlights();
  if (!q) { search_mtc.textContent = ''; return; }
  state.searchResults = [];
  state.searchIdx = 0;

  var paneEl = focusedPaneEl();
  var bodies = paneEl
    ? [paneEl.querySelector('.pbody')]
    : Array.from(ws_el.querySelectorAll('.pbody'));

  bodies.forEach(function(body) {
    if (!body) return;
    Array.from(body.querySelectorAll('.line')).forEach(function(lineEl) {
      if (highlightLine(lineEl, q)) {
        state.searchResults.push({body: body, lineEl: lineEl});
      }
    });
  });

  search_mtc.textContent = state.searchResults.length
    ? (state.searchIdx+1) + '/' + state.searchResults.length
    : 'no matches';
  if (state.searchResults.length) scrollToMatch(0);
}

function highlightLine(lineEl, q) {
  var text = lineEl.textContent;
  if (!text.toLowerCase().includes(q.toLowerCase())) return false;
  lineEl.classList.add('match');
  return true;
}

function clearSearchHighlights() {
  ws_el.querySelectorAll('.line.match,.line.current').forEach(function(el){
    el.classList.remove('match','current');
  });
  state.searchResults = [];
  search_mtc.textContent = '';
}

function navigateSearch(delta) {
  if (!state.searchResults.length) return;
  var prev = state.searchResults[state.searchIdx];
  if (prev) prev.lineEl.classList.remove('current');
  state.searchIdx = (state.searchIdx + delta + state.searchResults.length) % state.searchResults.length;
  scrollToMatch(state.searchIdx);
  search_mtc.textContent = (state.searchIdx+1) + '/' + state.searchResults.length;
}

function scrollToMatch(idx) {
  var m = state.searchResults[idx];
  if (!m) return;
  m.lineEl.classList.add('current');
  m.lineEl.scrollIntoView({block:'nearest'});
}

// ═══════════════════════════════════════════════════════════════════════════
// Context menu
// ═══════════════════════════════════════════════════════════════════════════
var ctxPaneID = null;

function showCtxMenu(x, y, id) {
  ctxPaneID = id;
  ctx_menu.style.left = x + 'px';
  ctx_menu.style.top  = y + 'px';
  ctx_menu.classList.add('visible');
}

function closeCtxMenu() {
  ctx_menu.classList.remove('visible');
  ctxPaneID = null;
}

ctx_menu.addEventListener('click', function(e) {
  var item = e.target.closest('.ctx-item');
  if (!item) return;
  closeCtxMenu();
  dispatch(item.dataset.action, ctxPaneID);
});

document.addEventListener('mousedown', function(e) {
  if (!ctx_menu.contains(e.target)) closeCtxMenu();
});

// ═══════════════════════════════════════════════════════════════════════════
// Keyboard dispatch
// ═══════════════════════════════════════════════════════════════════════════

// Build a lookup table from bindings config.
// Multi-key sequences (e.g. "g t") are stored as "g" -> {"t" -> action}.
function buildKeyTable(bindingsMap) {
  var table = {};  // key -> action | {subkey -> action}
  Object.keys(bindingsMap).forEach(function(action) {
    var keys = bindingsMap[action];
    keys.forEach(function(keyStr) {
      keyStr = keyStr.trim();
      if (keyStr.includes(' ')) {
        // Multi-key sequence.
        var parts = keyStr.split(' ');
        var first = parts[0], second = parts.slice(1).join(' ');
        if (!table[first]) table[first] = {};
        if (typeof table[first] === 'string') {
          // Conflict: single key already bound. Prefer the single.
          return;
        }
        table[first][second] = action;
      } else {
        table[keyStr] = action;
      }
    });
  });
  return table;
}

var keyTable = {};

function evtKey(e) {
  // Normalise to a consistent string.
  var k = e.key;
  if (e.ctrlKey && k.length === 1) k = 'Control+' + k;
  if (e.ctrlKey && k === 'c') k = 'Control+c';
  if (e.shiftKey && k === 'Tab') k = 'Shift+Tab';
  return k;
}

document.addEventListener('keydown', function(e) {
  // Let prompt / search inputs handle their own keys.
  if (overlay.classList.contains('visible')) return;
  if (state.mode === 'search') {
    if (e.key === 'Escape' || (e.ctrlKey && e.key === 'c')) {
      e.preventDefault(); closeSearch(); return;
    }
    if (e.key === 'Enter') { e.preventDefault(); navigateSearch(1); return; }
    // Let the search input receive the character.
    return;
  }

  var k = evtKey(e);

  // Check if we're mid-sequence.
  if (state.keySeq.length > 0) {
    var prev = state.keySeq.join(' ');
    var subtable = keyTable[prev];
    if (subtable && typeof subtable === 'object') {
      var action = subtable[k];
      if (action) {
        e.preventDefault();
        state.keySeq = [];
        dispatch(action);
        return;
      }
    }
    // Unrecognised continuation — cancel sequence and reprocess.
    state.keySeq = [];
  }

  var binding = keyTable[k];
  if (!binding) return;

  if (typeof binding === 'string') {
    e.preventDefault();
    dispatch(binding);
  } else if (typeof binding === 'object') {
    // Start of multi-key sequence.
    e.preventDefault();
    state.keySeq = [k];
  }
});

// ═══════════════════════════════════════════════════════════════════════════
// WebSocket client
// ═══════════════════════════════════════════════════════════════════════════
var ws;
var reconnectQueue = [];
var RECONNECT_QUEUE_LEN = 1000;

function connect() {
  var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(proto + '//' + location.host + API + '/ws', [WS_PROTO]);

  ws.onopen = function() {
    s_conn.textContent = 'connected';
    s_conn.className = '';
  };

  ws.onclose = function() {
    s_conn.textContent = 'reconnecting…';
    s_conn.className = 'warn';
    setTimeout(connect, RECONNECT_MS);
  };

  ws.onerror = function() {
    s_conn.textContent = 'error';
    s_conn.className = 'err';
  };

  ws.onmessage = function(e) {
    var msg;
    try { msg = JSON.parse(e.data); } catch(_) { return; }

    switch(msg.type) {
      case 'reconnect':
        // Flush queued lines.
        reconnectQueue.forEach(function(item) {
          appendLine(item.id, item.line);
        });
        reconnectQueue = [];
        s_conn.textContent = 'connected';
        s_conn.className = '';
        break;

      case 'layout_update':
        if (msg.payload) {
          state.windows   = msg.payload.windows || [];
          state.activeWin = msg.payload.active_window || 0;
          renderLayout(state.windows, state.activeWin);
        }
        break;

      case 'window_focus':
        if (msg.payload && msg.payload.name) {
          var idx = state.windows.findIndex(function(w){ return w.name === msg.payload.name; });
          if (idx >= 0) {
            state.activeWin = idx;
            renderLayout(state.windows, state.activeWin);
          }
        }
        break;

      case 'pane_zoom':
        if (msg.payload && msg.payload.target_id) {
          dispatch('pane.zoom', msg.payload.target_id);
        }
        break;

      case 'highlight_set':
        // Apply a named highlight profile to a pane. Store profile rules and
        // re-highlight pane content on next render.
        if (msg.target_id && msg.profile && msg.profile.rules) {
          state.highlights = state.highlights || {};
          state.highlights[msg.target_id] = msg.profile.rules;
          applyHighlightProfile(msg.target_id, msg.profile.rules);
        }
        break;

      case 'scrollback_ctl':
        // sc: control actions: clear, top, bottom.
        if (msg.target_id && msg.action) {
          var el = document.getElementById('pane-' + msg.target_id);
          if (!el) break;
          if (msg.action === 'clear') {
            el.innerHTML = '';
            state.scrollback = state.scrollback || {};
            state.scrollback[msg.target_id] = [];
          } else if (msg.action === 'top') {
            el.scrollTop = 0;
          } else if (msg.action === 'bottom') {
            el.scrollTop = el.scrollHeight;
          }
        }
        break;

      case 'image_render':
        if (msg.target_id && msg.data) {
          renderImage(msg.target_id, msg.data, msg.scale || 'fit');
        }
        break;

      case 'svg_render':
        if (msg.target_id && msg.data) {
          renderSVG(msg.target_id, msg.data, msg.scale || 'fit');
        }
        break;

      case 'pane_scale':
        if (msg.target_id && msg.scale) {
          setPaneScale(msg.target_id, msg.scale);
        }
        break;

      case 'formatter_set':
        if (msg.target_id !== undefined) {
          state.paneFormatter = state.paneFormatter || {};
          state.paneFormatter[msg.target_id] = msg.formatter || 'text';
        }
        break;

      default:
        // Stream line: {target_id, line}
        if (msg.target_id !== undefined && msg.line !== undefined) {
          if (ws.readyState !== WebSocket.OPEN) {
            if (reconnectQueue.length < RECONNECT_QUEUE_LEN) {
              reconnectQueue.push({id: msg.target_id, line: msg.line});
            }
          } else {
            appendLine(msg.target_id, msg.line);
          }
        }
        break;
    }
  };
}

// ═══════════════════════════════════════════════════════════════════════════
// Session reload
// ═══════════════════════════════════════════════════════════════════════════
function reloadSession() {
  return get('/session').then(function(d) {
    state.windows   = d.windows   || [];
    state.activeWin = d.active_window || 0;
    renderLayout(state.windows, state.activeWin);
  }).catch(console.error);
}

// ═══════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════
function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

// ═══════════════════════════════════════════════════════════════════════════
// Startup
// ═══════════════════════════════════════════════════════════════════════════
Promise.all([
  get('/bindings'),
  get('/session')
]).then(function(results) {
  var bindingsMap = results[0] || {};
  var session     = results[1] || {};

  keyTable        = buildKeyTable(bindingsMap);
  state.windows   = session.windows   || [];
  state.activeWin = session.active_window || 0;
  renderLayout(state.windows, state.activeWin);

}).catch(function() {
  // Server may not have auth; still connect WS.
}).finally(function() {
  connect();
});

// Search input wiring.
search_inp.addEventListener('input', function() {
  runSearch(search_inp.value);
});
search_inp.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') { e.stopPropagation(); closeSearch(); }
  if (e.key === 'Enter')  { e.stopPropagation(); navigateSearch(1); }
  if (e.key === 'n')      { e.stopPropagation(); navigateSearch(1); }
  if (e.key === 'N')      { e.stopPropagation(); navigateSearch(-1); }
});

// CSV table sort — click a <th> to sort ascending, click again for descending.
document.addEventListener('click', function(e) {
  var th = e.target.closest('th');
  if (!th) return;
  var table = th.closest('table');
  if (!table || !table.classList.contains('rdw-csv-table')) return;

  var tbody = table.querySelector('tbody');
  if (!tbody) return;

  var col = Array.from(th.parentNode.children).indexOf(th);
  var asc  = th.dataset.sortDir !== 'asc';

  Array.from(th.parentNode.querySelectorAll('th')).forEach(function(h) {
    delete h.dataset.sortDir;
    h.classList.remove('sort-asc', 'sort-desc');
  });

  th.dataset.sortDir = asc ? 'asc' : 'desc';
  th.classList.add(asc ? 'sort-asc' : 'sort-desc');

  var rows = Array.from(tbody.querySelectorAll('tr'));
  rows.sort(function(a, b) {
    var av = (a.children[col] || {}).textContent || '';
    var bv = (b.children[col] || {}).textContent || '';
    var an = parseFloat(av), bn = parseFloat(bv);
    var cmp = (!isNaN(an) && !isNaN(bn)) ? (an - bn) : av.localeCompare(bv);
    return asc ? cmp : -cmp;
  });

  rows.forEach(function(r) { tbody.appendChild(r); });
});

})();
</script>
</body>
</html>`)
