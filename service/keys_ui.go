package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const keysPageHTML = `<!DOCTYPE html>
<html lang="vi">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>claude2api - Quản lý tài khoản</title>
<style>
  :root {
    --bg: #0f1117; --card: #1a1d27; --border: #2a2e3f;
    --text: #e6e8ef; --muted: #8b90a3; --accent: #d97757;
    --ok: #4ade80; --warn: #fbbf24; --err: #f87171;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: 'Segoe UI', system-ui, sans-serif;
    background: var(--bg); color: var(--text);
    min-height: 100vh; padding: 2rem;
  }
  .container { max-width: 760px; margin: 0 auto; }
  h1 { font-size: 1.5rem; margin-bottom: .25rem; }
  .subtitle { color: var(--muted); font-size: .9rem; margin-bottom: 2rem; }
  .card {
    background: var(--card); border: 1px solid var(--border);
    border-radius: 12px; padding: 1.25rem; margin-bottom: 1.5rem;
  }
  .card h2 { font-size: 1rem; margin-bottom: 1rem; color: var(--muted); font-weight: 600;
    text-transform: uppercase; letter-spacing: .05em; }
  table { width: 100%; border-collapse: collapse; }
  th { text-align: left; font-size: .75rem; color: var(--muted); padding: .5rem;
    text-transform: uppercase; letter-spacing: .05em; border-bottom: 1px solid var(--border); }
  td { padding: .65rem .5rem; border-bottom: 1px solid var(--border); font-size: .9rem; }
  tr:last-child td { border-bottom: none; }
  code {
    background: var(--bg); padding: .2rem .45rem; border-radius: 6px;
    font-family: Consolas, monospace; font-size: .82rem; word-break: break-all;
  }
  .badge {
    display: inline-block; padding: .15rem .6rem; border-radius: 999px;
    font-size: .75rem; font-weight: 600;
  }
  .badge.ok { background: rgba(74,222,128,.15); color: var(--ok); }
  .badge.limited { background: rgba(251,191,36,.15); color: var(--warn); }
  button {
    cursor: pointer; border: none; border-radius: 8px;
    padding: .45rem .9rem; font-size: .85rem; font-weight: 600;
    transition: opacity .15s;
  }
  button:hover { opacity: .85; }
  .btn-danger { background: rgba(248,113,113,.15); color: var(--err); }
  .btn-primary { background: var(--accent); color: #fff; width: 100%; padding: .7rem; font-size: .95rem; }
  input[type=text], textarea {
    width: 100%; background: var(--bg); border: 1px solid var(--border);
    border-radius: 8px; padding: .7rem .9rem; color: var(--text);
    font-family: Consolas, monospace; font-size: .85rem; margin-bottom: .75rem;
  }
  input[type=text]:focus, textarea:focus { outline: none; border-color: var(--accent); }
  textarea { min-height: 120px; resize: vertical; }
  .msg { margin-top: .75rem; font-size: .85rem; min-height: 1.2em; }
  .msg.error { color: var(--err); }
  .msg.success { color: var(--ok); }
  .hint { color: var(--muted); font-size: .8rem; margin-top: .5rem; line-height: 1.5; }
  a { color: var(--accent); text-decoration: none; }
  .topbar { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 1.5rem; }
  select {
    width: 100%; background: var(--bg); border: 1px solid var(--border);
    border-radius: 8px; padding: .6rem .8rem; color: var(--text);
    font-size: .85rem; margin-bottom: .75rem;
  }
  select:focus { outline: none; border-color: var(--accent); }
  .gw-row { display: flex; gap: .75rem; align-items: stretch; }
  .gw-row select { flex: 1; margin-bottom: 0; }
  .gw-row button { white-space: nowrap; background: var(--bg); color: var(--text);
    border: 1px solid var(--border); }
</style>
</head>
<body>
<div class="container">
  <div class="topbar">
    <div>
      <h1>Quản lý tài khoản (cookie)</h1>
      <div class="subtitle">claude2api &middot; <a href="/health">health</a></div>
    </div>
  </div>

  <div class="card">
    <h2>Tài khoản</h2>
    <table id="keysTable">
      <thead><tr><th>Tên acc</th><th>Trạng thái</th><th>Gateway</th><th></th></tr></thead>
      <tbody id="keysBody"></tbody>
    </table>
    <div style="margin-top:.75rem; display:flex; gap:.75rem; align-items:center;">
      <button id="checkBtn" onclick="checkKeys('')" style="background:rgba(217,119,87,.15); color:var(--accent);">Kiểm tra tất cả (claude.ai)</button>
      <span class="hint" id="checkStatus"></span>
    </div>
    <div class="msg" id="listMsg"></div>
    <div class="hint" style="margin-top:.5rem;">Chọn radio ở cột Gateway để đặt tài khoản dùng cho giao diện claude.ai tại cổng local. Để trống (không chọn) = tự chọn tài khoản có cookie.</div>
  </div>

  <div class="card">
    <h2>Thêm tài khoản (dán cookie)</h2>
    <textarea id="newCookie" placeholder='Dán JSON mảng cookie (EditThisCookie/Cookie-Editor export) tại đây' autocomplete="off"></textarea>
    <button class="btn-primary" onclick="addAccount()">Thêm vào pool</button>
    <div class="msg" id="addMsg"></div>
    <div class="hint">Cookie được lưu vào <code>accounts.json</code> và áp dụng tức thì. Tên acc tự lấy từ claude.ai.</div>
  </div>

  <div class="card">
    <h2>5 request mới nhất</h2>
    <table id="recentTable">
      <thead><tr><th>Thời gian</th><th>Tài khoản</th><th>Model</th><th style="text-align:right;">In ↑</th><th style="text-align:right;">Out ↓</th><th>Kết quả</th><th>Nội dung</th></tr></thead>
      <tbody id="recentBody">
        <tr><td colspan="7" style="color:var(--muted);">Chưa có request nào</td></tr>
      </tbody>
    </table>
  </div>
</div>

<script>
const API_KEY = new URLSearchParams(location.search).get('apikey') || localStorage.getItem('c2a_apikey') || '';
if (API_KEY) localStorage.setItem('c2a_apikey', API_KEY);

const headers = { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + API_KEY };
// results from the last claude.ai validation, keyed by full session key
const checkResults = {};

function statusBadge(k) {
  const cr = checkResults[k.full];
  if (cr) {
    if (cr.status === 'valid')
      return '<span class="badge ok" title="' + escapeHtml(cr.detail || '') + '">Hoạt động</span>';
    if (cr.status === 'rate_limited')
      return '<span class="badge limited" title="' + escapeHtml(cr.detail || '') + '">Hết hạn mức</span>';
    return '<span class="badge limited" style="background:rgba(248,113,113,.15); color:var(--err);" title="' + escapeHtml(cr.detail || '') + '">Lỗi/Hết hiệu lực</span>';
  }
  return '<span class="badge limited" style="background:rgba(139,144,163,.15); color:var(--muted);">Chưa kiểm tra</span>';
}

function label(k) {
  return k.displayName || k.masked || k.full.slice(0, 24) + '...';
}

let gatewayKey = '';

async function loadKeys() {
  const res = await fetch('/keys/api', { headers });
  if (res.status === 401) {
    document.getElementById('listMsg').textContent = 'Thiếu API key — mở trang dạng /keys?apikey=<APIKEY của bạn>';
    document.getElementById('listMsg').className = 'msg error';
    return;
  }
  const data = await res.json();
  const tbody = document.getElementById('keysBody');
  tbody.innerHTML = '';
  for (const k of data.keys) {
    const tr = document.createElement('tr');
    const checked = k.full === gatewayKey ? ' checked' : '';
    tr.innerHTML =
      '<td><code>' + escapeHtml(label(k)) + '</code></td>' +
      '<td>' + statusBadge(k) + '</td>' +
      '<td style="text-align:center;"><input type="radio" name="gw" value="' + escapeHtml(k.full) + '"' + checked + ' onchange="saveGatewayKey(this.value)"></td>' +
      '<td style="text-align:right; white-space:nowrap;">' +
        '<button class="btn-danger" style="margin-right:.4rem;" onclick="checkKeys(\'' + escapeHtml(k.full) + '\')">Kiểm tra</button>' +
        '<button class="btn-danger" onclick="deleteAccount(\'' + escapeHtml(k.full) + '\')">Xóa</button>' +
      '</td>';
    tbody.appendChild(tr);
  }

  // keep the gateway selection in sync with the server once on load
  const gwRes = await fetch('/keys/api/gateway', { headers });
  if (gwRes.ok) {
    const gwData = await gwRes.json();
    if (!gatewayKey) gatewayKey = gwData.gatewayKey || '';
    const radio = document.querySelector('input[name="gw"][value="' + cssEscape(gatewayKey) + '"]');
    if (radio) radio.checked = true;
  }
}

function cssEscape(s) { return s.replace(/"/g, '\\"'); }

function fmt(n) { return String(n || 0).replace(/\B(?=(\d{3})+(?!\d))/g, ','); }

async function saveGatewayKey(key) {
  gatewayKey = key || '';
  const res = await fetch('/keys/api/gateway', {
    method: 'POST', headers,
    body: JSON.stringify({ key: gatewayKey })
  });
  const data = await res.json();
  const msg = document.getElementById('listMsg');
  msg.textContent = data.message || data.error;
  msg.className = res.ok ? 'msg success' : 'msg error';
}

function escapeHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

async function addAccount() {
  const cookie = document.getElementById('newCookie').value.trim();
  const msg = document.getElementById('addMsg');
  if (!cookie) { msg.textContent = 'Dán JSON cookie trước'; msg.className = 'msg error'; return; }
  const res = await fetch('/keys/api/add', {
    method: 'POST', headers,
    body: JSON.stringify({ cookie })
  });
  const data = await res.json();
  msg.textContent = data.message || data.error;
  msg.className = res.ok ? 'msg success' : 'msg error';
  if (res.ok) {
    document.getElementById('newCookie').value = '';
  }
  loadKeys();
}

async function deleteAccount(key) {
  if (!confirm('Xoá tài khoản này khỏi pool?')) return;
  const msg = document.getElementById('listMsg');
  const res = await fetch('/keys/api/delete', {
    method: 'POST', headers,
    body: JSON.stringify({ key })
  });
  const data = await res.json();
  msg.textContent = data.message || data.error;
  msg.className = res.ok ? 'msg success' : 'msg error';
  delete checkResults[key];
  loadKeys();
}

async function loadRecent() {
  try {
    const res = await fetch('/keys/api/recent', { headers });
    if (!res.ok) return;
    const data = await res.json();
    const tbody = document.getElementById('recentBody');
    const rows = (data.requests || []).map(function(r) {
      const t = new Date(r.time);
      const hh = String(t.getHours()).padStart(2, '0') + ':' +
                 String(t.getMinutes()).padStart(2, '0') + ':' +
                 String(t.getSeconds()).padStart(2, '0');
      const badge = r.ok
        ? '<span class="badge ok">' + r.status + '</span>'
        : '<span class="badge limited" style="background:rgba(248,113,113,.15); color:var(--err);">' + r.status + '</span>';
      return '<tr><td style="white-space:nowrap;">' + hh + '</td>' +
        '<td><code>' + escapeHtml(r.account) + '</code></td>' +
        '<td><code>' + escapeHtml(r.model) + '</code></td>' +
        '<td style="text-align:right; white-space:nowrap;">' + fmt(r.promptTokens) + ' <span style="color:var(--ok);">↑</span></td>' +
        '<td style="text-align:right; white-space:nowrap;">' + fmt(r.completionTokens) + ' <span style="color:var(--accent);">↓</span></td>' +
        '<td>' + badge + (r.ms ? ' <span style="color:var(--muted);">' + r.ms + 'ms</span>' : '') + '</td>' +
        '<td style="color:var(--muted);">' + escapeHtml(r.preview) + '</td></tr>';
    });
    tbody.innerHTML = rows.length
      ? rows.join('')
      : '<tr><td colspan="7" style="color:var(--muted);">Chưa có request nào</td></tr>';
  } catch (e) { /* dashboard keeps working when the endpoint is unreachable */ }
}

async function checkKeys(key) {
  const statusEl = document.getElementById('checkStatus');
  const btn = document.getElementById('checkBtn');
  btn.disabled = true;
  btn.style.opacity = '.5';
  statusEl.textContent = key ? 'Đang kiểm tra tài khoản...' : 'Đang kiểm tra tất cả (có thể mất vài giây)...';
  try {
    const res = await fetch('/keys/api/check', {
      method: 'POST', headers,
      body: JSON.stringify(key ? { key } : {})
    });
    const data = await res.json();
    if (!res.ok) {
      statusEl.textContent = data.error || 'Kiểm tra thất bại';
      return;
    }
    let ok = 0, limited = 0, bad = 0;
    for (const r of data.results) {
      checkResults[r.key] = r;
      if (r.status === 'valid') ok++;
      else if (r.status === 'rate_limited') limited++;
      else bad++;
    }
    statusEl.textContent = 'Kết quả: ' + ok + ' hoạt động, ' + limited + ' hết hạn mức, ' + bad + ' lỗi/hết hiệu lực';
    loadKeys();
  } catch (e) {
    statusEl.textContent = 'Lỗi kết nối: ' + e.message;
  } finally {
    btn.disabled = false;
    btn.style.opacity = '';
  }
}

loadKeys();
loadRecent();
setInterval(loadKeys, 15000);
setInterval(loadRecent, 5000);
</script>
</body>
</html>`

// KeysPageHandler serves the account/cookie management UI
func KeysPageHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(keysPageHTML))
}
