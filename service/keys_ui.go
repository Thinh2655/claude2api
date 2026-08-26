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
<title>claude2api - Quản lý Keys</title>
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
  .container { max-width: 720px; margin: 0 auto; }
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
  input[type=text] {
    width: 100%; background: var(--bg); border: 1px solid var(--border);
    border-radius: 8px; padding: .7rem .9rem; color: var(--text);
    font-family: Consolas, monospace; font-size: .85rem; margin-bottom: .75rem;
  }
  input[type=text]:focus { outline: none; border-color: var(--accent); }
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
      <h1>Quản lý Session Keys</h1>
      <div class="subtitle">claude2api &middot; <a href="/health">health</a></div>
    </div>
  </div>

  <div class="card">
    <h2>Danh sách keys</h2>
    <table id="keysTable">
      <thead><tr><th>Session Key</th><th>Trạng thái</th><th></th></tr></thead>
      <tbody id="keysBody"></tbody>
    </table>
    <div style="margin-top:.75rem; display:flex; gap:.75rem; align-items:center;">
      <button id="checkBtn" onclick="checkKeys('')" style="background:rgba(217,119,87,.15); color:var(--accent);">Kiểm tra tất cả (claude.ai)</button>
      <span class="hint" id="checkStatus"></span>
    </div>
    <div class="msg" id="listMsg"></div>
  </div>

  <div class="card">
    <h2>Gateway claude.ai sử dụng key</h2>
    <div class="gw-row">
      <select id="gatewayKeySelect"><option value="">— Tự chọn (ưu tiên key còn hạn mức) —</option></select>
      <button onclick="saveGatewayKey()">Lưu</button>
    </div>
    <div class="msg" id="gwMsg"></div>
    <div class="hint">Key được chọn sẽ dùng cho giao diện claude.ai tại cổng local. Nếu để trống, gateway tự xoay sang key còn hạn mức.</div>
  </div>

  <div class="card">
    <h2>Thêm key mới</h2>
    <input type="text" id="newKey" placeholder="sk-ant-sid02-... (có thể dán nhiều key cách nhau bằng dấu phẩy)" autocomplete="off">
    <button class="btn-primary" onclick="addKey()">Thêm vào pool</button>
    <div class="msg" id="addMsg"></div>
    <div class="hint">Key được lưu ngay vào file .env và áp dụng tức thì, không cần khởi động lại server.</div>
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
    tr.innerHTML =
      '<td><code>' + escapeHtml(k.full) + '</code></td>' +
      '<td>' + statusBadge(k) + '</td>' +
      '<td style="text-align:right; white-space:nowrap;">' +
        '<button class="btn-danger" style="margin-right:.4rem;" onclick="checkKeys(\'' + escapeHtml(k.full) + '\')">Kiểm tra</button>' +
        '<button class="btn-danger" onclick="deleteKey(\'' + escapeHtml(k.full) + '\')">Xóa</button>' +
      '</td>';
    tbody.appendChild(tr);
  }

  // refresh gateway dropdown only when the key list changed, so an
  // in-progress selection is not wiped by the periodic refresh
  const gwRes = await fetch('/keys/api/gateway', { headers });
  if (gwRes.ok) {
    const gwData = await gwRes.json();
    const sel = document.getElementById('gatewayKeySelect');
    const signature = data.keys.map(k => k.full).join('|');
    const userPicked = sel.dataset.userPicked === '1' && [...sel.options].some(o => o.value === sel.value);
    if (sel.dataset.signature !== signature) {
      const current = gwData.gatewayKey || '';
      sel.innerHTML = '<option value="">— Tự chọn (ưu tiên key còn hạn mức) —</option>';
      for (const k of data.keys) {
        const opt = document.createElement('option');
        opt.value = k.full;
        opt.textContent = k.full.slice(0, 28) + '...' + k.full.slice(-7);
        if (k.full === current) opt.selected = true;
        sel.appendChild(opt);
      }
      sel.dataset.signature = signature;
    }
    void userPicked;
  }
}

document.addEventListener('change', e => {
  if (e.target && e.target.id === 'gatewayKeySelect') {
    e.target.dataset.userPicked = '1';
  }
});

async function saveGatewayKey() {
  const msg = document.getElementById('gwMsg');
  const key = document.getElementById('gatewayKeySelect').value;
  const res = await fetch('/keys/api/gateway', {
    method: 'POST', headers,
    body: JSON.stringify({ key })
  });
  const data = await res.json();
  msg.textContent = data.message || data.error;
  msg.className = res.ok ? 'msg success' : 'msg error';
}

function escapeHtml(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

async function addKey() {
  const key = document.getElementById('newKey').value.trim();
  const msg = document.getElementById('addMsg');
  if (!key) { msg.textContent = 'Nhập key trước'; msg.className = 'msg error'; return; }
  const res = await fetch('/keys/api/add', {
    method: 'POST', headers,
    body: JSON.stringify({ key })
  });
  const data = await res.json();
  msg.textContent = data.message || data.error;
  msg.className = res.ok ? 'msg success' : 'msg error';
  if (res.ok) document.getElementById('newKey').value = '';
  loadKeys();
}

async function deleteKey(key) {
  if (!confirm('Xóa key này khỏi pool?')) return;
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

async function checkKeys(key) {
  const statusEl = document.getElementById('checkStatus');
  const btn = document.getElementById('checkBtn');
  btn.disabled = true;
  btn.style.opacity = '.5';
  statusEl.textContent = key ? 'Đang kiểm tra key...' : 'Đang kiểm tra tất cả keys (có thể mất vài giây)...';
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
setInterval(loadKeys, 15000);
</script>
</body>
</html>`

// KeysPageHandler serves the key management UI
func KeysPageHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(keysPageHTML))
}
