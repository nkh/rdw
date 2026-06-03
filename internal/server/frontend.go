package server

// frontendHTML is the placeholder single-page application served at /.
// Phase 4 will replace this with the full browser UI compiled into the binary.
var frontendHTML = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>rdw</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: #1e1e1e; color: #d4d4d4;
      font-family: 'Cascadia Code', 'Fira Code', monospace;
      font-size: 13px; display: flex; flex-direction: column; height: 100vh;
    }
    #header {
      background: #2d2d2d; padding: 4px 12px;
      border-bottom: 1px solid #3c3c3c;
      display: flex; align-items: center; gap: 4px; min-height: 28px;
      flex-shrink: 0;
    }
    #header .rdw-logo { color: #0078d4; font-weight: bold; margin-right: 12px; }
    .win-tab {
      cursor: pointer; padding: 2px 10px; border-radius: 3px;
      border: 1px solid transparent; user-select: none;
      transition: background 0.1s;
    }
    .win-tab:hover  { background: #3c3c3c; }
    .win-tab.active { background: #0078d4; color: #fff; border-color: #005a9e; }
    #workspace {
      flex: 1; display: grid; grid-template-columns: 1fr;
      grid-template-rows: 1fr; overflow: hidden;
    }
    .pane {
      display: flex; flex-direction: column;
      border: 1px solid #3c3c3c; overflow: hidden;
    }
    .pane-header {
      background: #252526; padding: 2px 8px;
      font-size: 11px; color: #858585;
      border-bottom: 1px solid #3c3c3c; flex-shrink: 0;
    }
    .pane-body {
      flex: 1; overflow-y: auto; padding: 4px 8px;
      white-space: pre-wrap; word-break: break-all; line-height: 1.5;
    }
    .pane-body .line { display: block; }
    .pane-body .line:hover { background: rgba(255,255,255,0.04); }
    #statusbar {
      background: #007acc; color: #fff; font-size: 11px;
      padding: 1px 12px; display: flex; gap: 16px; flex-shrink: 0;
    }
    #statusbar .sep { opacity: 0.5; }
  </style>
</head>
<body>
<div id="header">
  <span class="rdw-logo">rdw</span>
  <span id="win-tabs"></span>
</div>
<div id="workspace">
  <div class="pane">
    <div class="pane-header" id="active-pane-label">no active pane</div>
    <div class="pane-body" id="pane-body"></div>
  </div>
</div>
<div id="statusbar">
  <span id="conn-status">connecting…</span>
  <span class="sep">|</span>
  <span id="line-count">0 lines</span>
  <span class="sep">|</span>
  <span>rdw-v1</span>
</div>

<script>
(function() {
  'use strict';

  var connStatus  = document.getElementById('conn-status');
  var winTabs     = document.getElementById('win-tabs');
  var paneLabel   = document.getElementById('active-pane-label');
  var paneBody    = document.getElementById('pane-body');
  var lineCount   = document.getElementById('line-count');

  var totalLines  = 0;
  var windows     = [];
  var activeWin   = 0;
  var MAX_LINES   = 10000;

  // ANSI colour escape parser (basic 16 + 256 + true-colour).
  function ansiToHtml(text) {
    return text
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/\x1b\[([0-9;]*)m/g, function(_, codes) {
        if (!codes || codes === '0') return '</span><span>';
        var parts = [];
        codes.split(';').forEach(function(c) {
          var n = parseInt(c, 10);
          if (n === 1)  parts.push('font-weight:bold');
          if (n === 3)  parts.push('font-style:italic');
          if (n === 4)  parts.push('text-decoration:underline');
          if (n >= 30 && n <= 37) parts.push('color:' + ansi16[n-30]);
          if (n >= 90 && n <= 97) parts.push('color:' + ansi16[n-82]);
          if (n >= 40 && n <= 47) parts.push('background:' + ansi16[n-40]);
        });
        return parts.length ? '<span style="' + parts.join(';') + '">' : '';
      });
  }
  var ansi16 = [
    '#000','#c00','#0c0','#cc0','#00c','#c0c','#0cc','#ccc',
    '#888','#f00','#0f0','#ff0','#00f','#f0f','#0ff','#fff'
  ];

  function appendLine(targetID, text) {
    // For now render all lines in the single pane body.
    if (paneLabel.textContent === 'no active pane') {
      paneLabel.textContent = targetID;
    }
    var el = document.createElement('span');
    el.className = 'line';
    el.innerHTML = ansiToHtml(text);
    paneBody.appendChild(el);

    // Trim to MAX_LINES.
    totalLines++;
    while (paneBody.children.length > MAX_LINES) {
      paneBody.removeChild(paneBody.firstChild);
    }

    lineCount.textContent = totalLines + ' lines';

    // Auto-scroll if near bottom.
    var body = paneBody;
    if (body.scrollHeight - body.scrollTop - body.clientHeight < 80) {
      body.scrollTop = body.scrollHeight;
    }
  }

  function renderWindowTabs(wins, active) {
    winTabs.innerHTML = '';
    wins.forEach(function(w, i) {
      var tab = document.createElement('span');
      tab.className = 'win-tab' + (i === active ? ' active' : '');
      tab.textContent = w.name;
      tab.addEventListener('click', function() {
        fetch('/api/v1/windows/' + encodeURIComponent(w.name) + '/focus', {
          method: 'POST',
          headers: authHeaders()
        });
      });
      winTabs.appendChild(tab);
    });
  }

  function authHeaders() {
    var token = sessionStorage.getItem('rdw_token') || '';
    return token ? { 'Authorization': 'Bearer ' + token } : {};
  }

  function connect() {
    var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    var url   = proto + '//' + location.host + '/api/v1/ws';
    var ws    = new WebSocket(url, ['rdw-v1']);

    ws.onopen = function() {
      connStatus.textContent = 'connected';
      connStatus.style.color = '#fff';
    };

    ws.onclose = function() {
      connStatus.textContent = 'reconnecting…';
      connStatus.style.color = '#ffcc00';
      setTimeout(connect, 2000);
    };

    ws.onerror = function() {
      connStatus.textContent = 'error';
      connStatus.style.color = '#f66';
    };

    ws.onmessage = function(e) {
      try {
        var msg = JSON.parse(e.data);
        if (msg.type === 'reconnect') {
          connStatus.textContent = 'connected';
          return;
        }
        if (msg.type === 'layout_update' && msg.payload) {
          windows  = msg.payload.windows  || [];
          activeWin = msg.payload.active_window || 0;
          renderWindowTabs(windows, activeWin);
          return;
        }
        if (msg.type === 'window_focus' && msg.payload) {
          windows.forEach(function(w, i) {
            if (w.name === msg.payload.name) activeWin = i;
          });
          renderWindowTabs(windows, activeWin);
          return;
        }
        if (msg.target_id && msg.line !== undefined) {
          appendLine(msg.target_id, msg.line);
        }
      } catch(ex) { /* ignore malformed */ }
    };
  }

  // Load session state on startup.
  fetch('/api/v1/session', { headers: authHeaders() })
    .then(function(r) { return r.json(); })
    .then(function(d) {
      windows   = d.windows  || [];
      activeWin = d.active_window || 0;
      renderWindowTabs(windows, activeWin);
    })
    .catch(function() {});

  connect();
})();
</script>
</body>
</html>`)
