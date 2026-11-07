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
  .badge.cooling { background: rgba(96,165,250,.15); color: #60a5fa; }
  button {
    cursor: pointer; border: none; border-radius: 8px;
    padding: .45rem .9rem; font-size: .85rem; font-weight: 600;
    transition: opacity .15s;
  }
  button:hover { opacity: .85; }
  .btn-danger { background: rgba(248,113,113,.15); color: var(--err); }
  .btn-primary { background: var(--accent); color: #fff; width: 100%; padding: .7rem; font-size: .95rem; }
  input[type=text], input[type=password], textarea {
    width: 100%; background: var(--bg); border: 1px solid var(--border);
    border-radius: 8px; padding: .7rem .9rem; color: var(--text);
    font-family: Consolas, monospace; font-size: .85rem; margin-bottom: .75rem;
  }
  input[type=text]:focus, input[type=password]:focus, textarea:focus { outline: none; border-color: var(--accent); }
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
  .mcp-form-grid { display: flex; gap: .75rem; align-items: flex-end; margin-bottom: .75rem; }
  .mcp-form-grid > div { flex: 1; min-width: 0; }
  .mcp-form-grid > div.mcp-url-field { flex: 2; }
  .mcp-form-grid input, .mcp-field select { margin-bottom: 0; }
  .mcp-field { margin-bottom: .75rem; }
  .mcp-field-label { display: block; font-size: .78rem; color: var(--muted); margin-bottom: .35rem; }
  .mcp-section { margin-top: .75rem; padding-top: .75rem; border-top: 1px solid var(--border); }
  .mcp-section-title { font-weight: 600; font-size: .88rem; margin-bottom: .35rem; }
  .mcp-header-row { display: flex; gap: .5rem; align-items: center; margin-top: .5rem; flex-wrap: wrap; }
  .mcp-header-row input[type=text], .mcp-header-row input[type=password] {
    margin-bottom: 0; flex: 1 1 120px;
  }
  .mcp-header-row .mcp-header-name { flex: 1 1 150px; }
  .mcp-header-row .mcp-header-value { flex: 2 1 180px; }
  .mcp-header-row button { padding: .4rem .6rem; }
  .mcp-required { display: flex; align-items: center; gap: .3rem; font-size: .8rem;
    color: var(--muted); white-space: nowrap; }
  .mcp-add-header { background: var(--bg); color: var(--text); border: 1px solid var(--border);
    margin-top: .5rem; }
  .mcp-add-header:disabled { opacity: .4; cursor: not-allowed; }
  .switch-row { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
  .switch { position: relative; display: inline-block; width: 46px; height: 26px; flex: none; }
  .switch input { opacity: 0; width: 0; height: 0; }
  .slider { position: absolute; cursor: pointer; inset: 0; background: var(--border);
    border-radius: 999px; transition: background .15s; }
  .slider:before { content: ""; position: absolute; height: 18px; width: 18px; left: 4px; top: 4px;
    background: #fff; border-radius: 50%; transition: transform .15s; }
  .switch input:checked + .slider { background: var(--ok); }
  .switch input:checked + .slider:before { transform: translateX(20px); }
  .switch input:disabled + .slider { opacity: .5; cursor: wait; }
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
      <thead><tr><th>Tên acc</th><th>Trạng thái</th><th>Gateway</th><th style="text-align:center;">Bật</th><th></th></tr></thead>
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
    <h2>Cài đặt</h2>
    <div class="switch-row">
      <div>
        <div>Hỗ trợ tool (function-calling)</div>
        <div class="hint">Forward tool client gửi lên model và trả <code>tool_calls</code> chuẩn OpenAI. Tắt = bỏ qua mọi tool trong request.</div>
      </div>
      <label class="switch">
        <input type="checkbox" id="toolsToggle" onchange="saveTools(this)">
        <span class="slider"></span>
      </label>
    </div>
    <div class="msg" id="toolsMsg"></div>
    <div class="switch-row" style="margin-top:1rem;">
      <div>
        <div>Chỉ nhận ảnh ở message cuối</div>
        <div class="hint">Bật = bỏ qua ảnh ở các message cũ, chỉ upload ảnh của message cuối cùng. Giảm upload/token khi chat dài có ảnh.</div>
      </div>
      <label class="switch">
        <input type="checkbox" id="lastImageToggle" onchange="saveLastImage(this)">
        <span class="slider"></span>
      </label>
    </div>
    <div class="msg" id="lastImageMsg"></div>
    <div class="switch-row" style="margin-top:1rem;">
      <div>
        <div>Hỗ trợ MCP (Model Context Protocol)</div>
        <div class="hint">Tự động nạp các công cụ MCP server đã cấu hình trên tài khoản vào request completion. Lưu vào cấu hình ENABLE_MCP.</div>
      </div>
      <label class="switch">
        <input type="checkbox" id="mcpToggle" onchange="saveMCP(this)">
        <span class="slider"></span>
      </label>
    </div>
    <div class="msg" id="mcpMsg"></div>
  </div>

  <div class="card">
    <h2>Đồng bộ MCP</h2>
    <div class="mcp-form-grid">
      <div>
        <label class="mcp-field-label" for="mcpSyncName">Tên MCP</label>
        <input type="text" id="mcpSyncName" placeholder="Tên MCP (vd: tool)" autocomplete="off" value="tool">
      </div>
      <div class="mcp-url-field">
        <label class="mcp-field-label" for="mcpSyncUrl">URL MCP</label>
        <input type="text" id="mcpSyncUrl" placeholder="URL MCP (vd: https://tool.kalei.site/mcp)" autocomplete="off" value="https://tool.kalei.site/mcp">
      </div>
    </div>
    <div class="mcp-field">
      <label class="mcp-field-label" for="mcpAuthMode">Xác thực</label>
      <select id="mcpAuthMode" onchange="updateMCPAuthUI()">
        <option value="auth_required">Đăng nhập ngay</option>
        <option value="defer_auth">Đăng nhập khi cần</option>
        <option value="no_auth" selected>Không đăng nhập</option>
      </select>
      <div class="hint" id="mcpAuthHint"></div>
    </div>
    <div id="mcpOAuthFields" style="display:none;">
      <div class="mcp-field">
        <label class="mcp-field-label" for="mcpRegistration">OAuth client</label>
        <select id="mcpRegistration" onchange="updateMCPAuthUI()">
          <option value="cimd">Dùng danh tính Claude (CIMD) - khuyến nghị</option>
          <option value="dcr">Đăng ký client tự động (DCR)</option>
          <option value="static_per_installation">Dùng OAuth client riêng</option>
        </select>
      </div>
      <div id="mcpStaticOAuthFields" style="display:none;">
        <div class="mcp-field">
          <label class="mcp-field-label" for="mcpClientId">OAuth client ID</label>
          <input type="text" id="mcpClientId" placeholder="Client ID" autocomplete="off">
        </div>
        <div class="mcp-field">
          <label class="mcp-field-label" for="mcpClientSecret">OAuth client secret</label>
          <input type="password" id="mcpClientSecret" placeholder="Client secret (không bắt buộc)" autocomplete="new-password">
        </div>
      </div>
    </div>
    <div class="mcp-section">
      <div class="mcp-section-title">Request headers</div>
      <div class="hint">Tối đa 4 header. Header có value sẽ được gửi kèm; header không có value vẫn được khai báo nếu không đánh dấu Bắt buộc.</div>
      <div id="mcpHeaderRows"></div>
      <button type="button" class="mcp-add-header" id="mcpAddHeaderBtn" onclick="addMCPHeader()">+ Thêm header</button>
      <div class="hint" id="mcpHeaderCount"></div>
    </div>
    <div style="display:flex; gap:.75rem; align-items:stretch; margin-top:1rem;">
      <button class="btn-primary" id="mcpSyncBtn" onclick="syncMCP()" style="flex:1;">Đồng bộ tất cả tài khoản</button>
      <button class="btn-danger" id="mcpDeleteBtn" onclick="deleteMCP()" style="white-space:nowrap;">Xóa MCP custom</button>
    </div>
    <div class="msg" id="mcpSyncMsg"></div>
    <div class="hint">Đồng bộ: xóa tất cả remote MCP custom cũ của từng tài khoản trong pool và thêm MCP mới với cấu hình xác thực/header bên trên (các tool luôn được bật Always Allow). Connector dùng OAuth có thể cần đăng nhập trên claude.ai sau khi tạo.<br>OAuth client ID/secret và giá trị header chỉ dùng cho lần đồng bộ này, không lưu vào .env.<br>Xóa MCP custom: xóa toàn bộ remote MCP custom của tất cả tài khoản, không thêm mới và không thể hoàn tác.</div>
    <div id="mcpSyncResults" style="margin-top:.75rem; font-size:.85rem;"></div>
  </div>

  <div class="card">
    <h2>Đồng bộ quyền riêng tư</h2>
    <div class="switch-row">
      <div>
        <div>Siêu dữ liệu vị trí</div>
        <div class="hint">Cho phép Claude sử dụng siêu dữ liệu vị trí gần đúng (thành phố/khu vực) để cải thiện trải nghiệm sản phẩm. Lưu vào cấu hình ENABLE_GEOLOCATION.</div>
      </div>
      <label class="switch">
        <input type="checkbox" id="geolocationToggle">
        <span class="slider"></span>
      </label>
    </div>
    <div class="switch-row" style="margin-top:1rem;">
      <div>
        <div>Help improve our AI models</div>
        <div class="hint">Cho phép sử dụng chat và phiên coding để huấn luyện/cải thiện mô hình AI của Anthropic. Lưu vào cấu hình ENABLE_GROVE.</div>
      </div>
      <label class="switch">
        <input type="checkbox" id="groveToggle">
        <span class="slider"></span>
      </label>
    </div>
    <div style="display:flex; gap:.75rem; align-items:stretch; margin-top:1rem;">
      <button class="btn-primary" id="privacySyncBtn" onclick="syncPrivacy()" style="flex:1;">Đồng bộ tất cả tài khoản</button>
    </div>
    <div class="msg" id="privacySyncMsg"></div>
    <div class="hint">Công tắc chỉ là giá trị đích; bấm Đồng bộ để áp dụng cho toàn bộ tài khoản trong pool. Tài khoản thêm mới sau đó cần đồng bộ lại.</div>
    <div id="privacySyncResults" style="margin-top:.75rem; font-size:.85rem;"></div>
  </div>

  <div class="card">
    <h2>Bộ nhớ</h2>
    <div class="switch-row">
      <div>
        <div>Tạo bộ nhớ từ đoạn chat</div>
        <div class="hint">Cho phép Claude tạo bộ nhớ từ các đoạn chat của bạn. Lưu vào cấu hình ENABLE_MEMORY.</div>
      </div>
      <label class="switch">
        <input type="checkbox" id="memoryToggle">
        <span class="slider"></span>
      </label>
    </div>
    <div style="display:flex; gap:.75rem; align-items:stretch; margin-top:1rem;">
      <button class="btn-primary" id="memorySyncBtn" onclick="syncMemory()" style="flex:1;">Đồng bộ tất cả tài khoản</button>
    </div>
    <div class="msg" id="memorySyncMsg"></div>
    <div class="hint">Công tắc chỉ là giá trị đích; bấm Đồng bộ để áp dụng cho toàn bộ tài khoản trong pool. Tắt chỉ dừng tạo bộ nhớ mới, không xóa bộ nhớ đã có. Tài khoản Team/Enterprise có thể bị quản trị viên tổ chức khóa.</div>
    <div id="memorySyncResults" style="margin-top:.75rem; font-size:.85rem;"></div>
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
      <thead><tr><th>Thời gian</th><th>Tài khoản</th><th>Model</th><th style="text-align:right;">In ↑</th><th style="text-align:right;">Out ↓</th><th>Kết quả</th><th style="text-align:right;">Ký tự</th><th>Nội dung</th></tr></thead>
      <tbody id="recentBody">
        <tr><td colspan="8" style="color:var(--muted);">Chưa có request nào</td></tr>
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

function fmtCountdown(sec) {
  sec = Math.max(0, Math.floor(sec || 0));
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
  const mm = String(m).padStart(2, '0'), ss = String(s).padStart(2, '0');
  if (h > 0) return h + ':' + mm + ':' + ss;
  return m + ':' + ss;
}

function statusBadge(k) {
  // Usage-limit lock from the server (chat flow or /check 429) takes priority:
  // it carries the live countdown, unlike the one-shot manual check result.
  if (k.limited) {
    const tip = (k.limitReason || "You've hit your limit") + (k.limitedUntil ? ' · mở lại lúc ' + k.limitedUntil : '');
    return '<span class="badge limited" title="' + escapeHtml(tip) + '">Tạm khóa <span class="countdown" data-rem="' +
      (k.limitedRemaining || 0) + '">' + fmtCountdown(k.limitedRemaining) + '</span></span>';
  }
  // Cooldown ngắn 10s sau mỗi request thành công (hoặc 30s sau fail): acc vẫn
  // dùng được nhưng bị đẩy xuống cuối round-robin để tránh limit.
  if (k.cooling) {
    return '<span class="badge cooling" title="Vừa dùng xong, nghỉ tránh limit — request tới sẽ ưu tiên acc khác">Nghỉ <span class="countdown" data-rem="' +
      (k.coolingRemaining || 0) + '">' + fmtCountdown(k.coolingRemaining) + '</span></span>';
  }
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

// Tick every .countdown badge each second so the "mở lại sau" timer counts
// down live between the 15s server refreshes. When one hits zero the lock has
// likely expired server-side, so refresh the list to pick up the new state.
function tickCountdowns() {
  let expired = false;
  document.querySelectorAll('.countdown').forEach(function(el) {
    let rem = parseInt(el.getAttribute('data-rem') || '0', 10);
    if (rem > 0) {
      rem -= 1;
      el.setAttribute('data-rem', String(rem));
      el.textContent = fmtCountdown(rem);
      if (rem <= 0) expired = true;
    }
  });
  if (expired) loadKeys();
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
  let limitedCount = 0;
  for (const k of data.keys) {
    if (k.limited) limitedCount++;
    const tr = document.createElement('tr');
    const checked = k.full === gatewayKey ? ' checked' : '';
    const locked = !!k.limited;
    const unusable = k.disabled || locked;
    let actions =
        '<button class="btn-danger" style="margin-right:.4rem;" onclick="checkKeys(\'' + escapeHtml(k.full) + '\')">Kiểm tra</button>';
    if (locked) {
      actions += '<button style="margin-right:.4rem; background:rgba(74,222,128,.15); color:var(--ok);" onclick="unlockAccount(\'' + escapeHtml(k.full) + '\')">Mở khóa</button>';
    }
    actions += '<button class="btn-danger" onclick="deleteAccount(\'' + escapeHtml(k.full) + '\')">Xóa</button>';
    tr.innerHTML =
      '<td><code' + (k.disabled ? ' style="opacity:.45; text-decoration:line-through;"' : '') + '>' + escapeHtml(label(k)) + '</code>' +
      (locked && k.limitedUntil ? '<div class="hint" style="margin-top:.2rem;">mở lại lúc ' + escapeHtml(k.limitedUntil) + '</div>' : '') + '</td>' +
      '<td>' + statusBadge(k) + '</td>' +
      '<td style="text-align:center;"><input type="radio" name="gw" value="' + escapeHtml(k.full) + '"' + checked + (unusable ? ' disabled' : '') + ' onchange="saveGatewayKey(this.value)"></td>' +
      '<td style="text-align:center;"><input type="checkbox"' + (k.disabled ? ' checked' : '') + ' onchange="toggleAccount(this, \'' + escapeHtml(k.full) + '\')"></td>' +
      '<td style="text-align:right; white-space:nowrap;">' + actions + '</td>';
    tbody.appendChild(tr);
  }
  const listMsg = document.getElementById('listMsg');
  if (limitedCount > 0 && !listMsg.textContent) {
    listMsg.textContent = limitedCount + ' tài khoản đang tạm khóa (limit) — request sẽ bỏ qua các acc này tới khi đếm ngược xong';
    listMsg.className = 'msg error';
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

async function loadTools() {
  const res = await fetch('/keys/api/tools', { headers });
  if (!res.ok) return;
  const data = await res.json();
  document.getElementById('toolsToggle').checked = !!data.enableTools;
}

async function saveTools(el) {
  const msg = document.getElementById('toolsMsg');
  el.disabled = true;
  try {
    const res = await fetch('/keys/api/tools', {
      method: 'POST', headers,
      body: JSON.stringify({ enabled: el.checked })
    });
    const data = await res.json();
    if (!res.ok) {
      el.checked = !el.checked;
      msg.textContent = data.error || 'Lưu thất bại';
      msg.className = 'msg error';
      return;
    }
    msg.textContent = 'Đã ' + (data.enableTools ? 'bật' : 'tắt') + ' hỗ trợ tool';
    msg.className = 'msg success';
  } catch (e) {
    el.checked = !el.checked;
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    el.disabled = false;
  }
}

async function loadLastImage() {
  const res = await fetch('/keys/api/last-image', { headers });
  if (!res.ok) return;
  const data = await res.json();
  document.getElementById('lastImageToggle').checked = !!data.onlyLastImage;
}

async function saveLastImage(el) {
  const msg = document.getElementById('lastImageMsg');
  el.disabled = true;
  try {
    const res = await fetch('/keys/api/last-image', {
      method: 'POST', headers,
      body: JSON.stringify({ enabled: el.checked })
    });
    const data = await res.json();
    if (!res.ok) {
      el.checked = !el.checked;
      msg.textContent = data.error || 'Lưu thất bại';
      msg.className = 'msg error';
      return;
    }
    msg.textContent = 'Đã ' + (data.onlyLastImage ? 'bật' : 'tắt') + ' chỉ nhận ảnh ở message cuối';
    msg.className = 'msg success';
  } catch (e) {
    el.checked = !el.checked;
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    el.disabled = false;
  }
}

async function loadMCP() {
  const res = await fetch('/keys/api/mcp', { headers });
  if (!res.ok) return;
  const data = await res.json();
  document.getElementById('mcpToggle').checked = !!data.enableMCP;
}

async function saveMCP(el) {
  const msg = document.getElementById('mcpMsg');
  el.disabled = true;
  try {
    const res = await fetch('/keys/api/mcp', {
      method: 'POST', headers,
      body: JSON.stringify({ enabled: el.checked })
    });
    const data = await res.json();
    if (!res.ok) {
      el.checked = !el.checked;
      msg.textContent = data.error || 'Lưu thất bại';
      msg.className = 'msg error';
      return;
    }
    msg.textContent = 'Đã ' + (data.enableMCP ? 'bật' : 'tắt') + ' hỗ trợ MCP';
    msg.className = 'msg success';
  } catch (e) {
    el.checked = !el.checked;
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    el.disabled = false;
  }
}

async function loadPrivacy() {
  const res = await fetch('/keys/api/privacy', { headers });
  if (!res.ok) return;
  const data = await res.json();
  document.getElementById('geolocationToggle').checked = !!data.geolocation;
  document.getElementById('groveToggle').checked = !!data.grove;
}

function setPrivacyBusy(busy) {
  const btn = document.getElementById('privacySyncBtn');
  btn.disabled = busy;
  btn.style.opacity = busy ? '.5' : '';
}

async function syncPrivacy() {
  const geolocation = document.getElementById('geolocationToggle').checked;
  const grove = document.getElementById('groveToggle').checked;
  const msg = document.getElementById('privacySyncMsg');
  const resDiv = document.getElementById('privacySyncResults');
  const detail = 'Siêu dữ liệu vị trí: ' + (geolocation ? 'BẬT' : 'TẮT') + '\nHelp improve our AI models: ' + (grove ? 'BẬT' : 'TẮT');
  const warning = grove ? '\n\nCẢNH BÁO: bật "Help improve our AI models" cho phép dùng chat/coding session để huấn luyện model.' : '';
  if (!confirm('Đồng bộ quyền riêng tư cho TOÀN BỘ tài khoản trong pool?\n\n' + detail + warning)) {
    return;
  }

  setPrivacyBusy(true);
  msg.textContent = 'Đang đồng bộ quyền riêng tư cho các tài khoản... Vui lòng đợi trong giây lát.';
  msg.className = 'msg';
  resDiv.innerHTML = '';

  try {
    const res = await fetch('/keys/api/privacy/sync', {
      method: 'POST', headers,
      body: JSON.stringify({ geolocation: geolocation, grove: grove })
    });
    const data = await res.json();
    if (!res.ok) {
      msg.textContent = data.message || data.error || 'Đồng bộ thất bại';
      msg.className = 'msg error';
      return;
    }
    msg.textContent = data.message || 'Đồng bộ hoàn tất';
    msg.className = 'msg success';
    renderSyncResults(data.results, 'privacySyncResults');
  } catch (e) {
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    setPrivacyBusy(false);
  }
}

async function loadMemory() {
  const res = await fetch('/keys/api/memory', { headers });
  if (!res.ok) return;
  const data = await res.json();
  document.getElementById('memoryToggle').checked = !!data.memory;
}

function setMemoryBusy(busy) {
  const btn = document.getElementById('memorySyncBtn');
  btn.disabled = busy;
  btn.style.opacity = busy ? '.5' : '';
}

async function syncMemory() {
  const enabled = document.getElementById('memoryToggle').checked;
  const msg = document.getElementById('memorySyncMsg');
  const resDiv = document.getElementById('memorySyncResults');
  const detail = 'Tạo bộ nhớ từ đoạn chat: ' + (enabled ? 'BẬT' : 'TẮT');
  const warning = enabled ? '\n\nCẢNH BÁO: bật bộ nhớ cho phép Claude lưu thông tin từ các cuộc chat và dùng lại trong các request sau.' : '';
  if (!confirm('Đồng bộ cài đặt bộ nhớ cho TOÀN BỘ tài khoản trong pool?\n\n' + detail + warning)) {
    return;
  }

  setMemoryBusy(true);
  msg.textContent = 'Đang đồng bộ bộ nhớ cho các tài khoản... Vui lòng đợi trong giây lát.';
  msg.className = 'msg';
  resDiv.innerHTML = '';

  try {
    const res = await fetch('/keys/api/memory/sync', {
      method: 'POST', headers,
      body: JSON.stringify({ enabled: enabled })
    });
    const data = await res.json();
    if (!res.ok) {
      msg.textContent = data.message || data.error || 'Đồng bộ thất bại';
      msg.className = 'msg error';
      return;
    }
    msg.textContent = data.message || 'Đồng bộ hoàn tất';
    msg.className = 'msg success';
    renderSyncResults(data.results, 'memorySyncResults');
  } catch (e) {
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    setMemoryBusy(false);
  }
}

function updateMCPAuthUI() {
  const modeEl = document.getElementById('mcpAuthMode');
  const regEl = document.getElementById('mcpRegistration');
  if (!modeEl || !regEl) return;
  const mode = modeEl.value;
  const oauth = mode !== 'no_auth';
  document.getElementById('mcpOAuthFields').style.display = oauth ? '' : 'none';
  document.getElementById('mcpStaticOAuthFields').style.display = oauth && regEl.value === 'static_per_installation' ? '' : 'none';

  const hint = document.getElementById('mcpAuthHint');
  if (mode === 'auth_required') {
    hint.textContent = 'Mỗi người dùng phải đăng nhập qua OAuth của server trước khi dùng tool.';
  } else if (mode === 'defer_auth') {
    hint.textContent = 'Claude kết nối trước, chỉ yêu cầu đăng nhập khi server cần. Phù hợp server có tool công khai.';
  } else {
    hint.textContent = 'Không dùng OAuth. Nếu khai báo request header, connector sẽ dùng static headers (ví dụ API key).';
  }
}

function addMCPHeader(name, value, required) {
  const rows = document.getElementById('mcpHeaderRows');
  if (!rows) return;
  if (rows.children.length >= 4) return;

  const row = document.createElement('div');
  row.className = 'mcp-header-row';

  const nameInput = document.createElement('input');
  nameInput.type = 'text';
  nameInput.className = 'mcp-header-name';
  nameInput.placeholder = 'Tên header (vd: X-Api-Key)';
  nameInput.maxLength = 64;
  nameInput.autocomplete = 'off';
  nameInput.value = name || '';

  const valueInput = document.createElement('input');
  valueInput.type = 'password';
  valueInput.className = 'mcp-header-value';
  valueInput.placeholder = 'Giá trị header';
  valueInput.autocomplete = 'new-password';
  valueInput.value = value || '';

  const requiredLabel = document.createElement('label');
  requiredLabel.className = 'mcp-required';
  const requiredInput = document.createElement('input');
  requiredInput.type = 'checkbox';
  requiredInput.className = 'mcp-header-required';
  requiredInput.checked = required !== false;
  requiredLabel.appendChild(requiredInput);
  requiredLabel.appendChild(document.createTextNode(' Bắt buộc'));

  const removeButton = document.createElement('button');
  removeButton.type = 'button';
  removeButton.className = 'btn-danger';
  removeButton.textContent = 'Xóa';
  removeButton.onclick = function() { removeMCPHeader(this); };

  row.appendChild(nameInput);
  row.appendChild(valueInput);
  row.appendChild(requiredLabel);
  row.appendChild(removeButton);
  rows.appendChild(row);
  updateMCPHeaderControls();
}

function removeMCPHeader(button) {
  const row = button.closest('.mcp-header-row');
  if (row) row.remove();
  updateMCPHeaderControls();
}

function updateMCPHeaderControls() {
  const rows = document.getElementById('mcpHeaderRows');
  const addBtn = document.getElementById('mcpAddHeaderBtn');
  const count = rows ? rows.children.length : 0;
  if (addBtn) addBtn.disabled = count >= 4;
  const hint = document.getElementById('mcpHeaderCount');
  if (hint) hint.textContent = count ? count + '/4 header' : 'Chưa có header nào';
}

function collectMCPHeaders() {
  const headers = [];
  document.querySelectorAll('#mcpHeaderRows .mcp-header-row').forEach(function(row) {
    const nameInput = row.querySelector('.mcp-header-name');
    const valueInput = row.querySelector('.mcp-header-value');
    const requiredInput = row.querySelector('.mcp-header-required');
    const name = (nameInput.value || '').trim();
    const value = (valueInput.value || '').trim();
    if (!name && !value) return;
    headers.push({ name: name, value: value, required: !!(requiredInput && requiredInput.checked) });
  });
  return headers;
}

function validateMCPHeaders(headers, authMode) {
  if (headers.length > 4) return 'Tối đa 4 request header';
  const seen = new Set();
  for (let i = 0; i < headers.length; i++) {
    const h = headers[i];
    if (!h.name) return 'Tên request header không được để trống';
    if (!/^[A-Za-z0-9_.-]{1,64}$/.test(h.name)) return 'Tên request header không hợp lệ: ' + h.name;
    const lower = h.name.toLowerCase();
    if (seen.has(lower)) return 'Request header bị trùng: ' + h.name;
    seen.add(lower);
    if (h.required && !h.value) return 'Header ' + h.name + ' bắt buộc phải có value';
    if (authMode !== 'no_auth' && lower === 'authorization') return 'Header Authorization do OAuth quản lý';
  }
  if (authMode === 'no_auth' && headers.length > 0 && headers.every(function(h) { return !h.value; })) {
    return 'Khi không dùng OAuth, cần ít nhất một request header có value';
  }
  return '';
}

function initMCPForm() {
  updateMCPAuthUI();
  const rows = document.getElementById('mcpHeaderRows');
  if (rows && rows.children.length === 0) addMCPHeader();
}

async function syncMCP() {
  const nameInput = document.getElementById('mcpSyncName');
  const urlInput = document.getElementById('mcpSyncUrl');
  const name = (nameInput.value || '').trim() || 'tool';
  const url = (urlInput.value || '').trim();
  const msg = document.getElementById('mcpSyncMsg');
  const resDiv = document.getElementById('mcpSyncResults');
  const authModeEl = document.getElementById('mcpAuthMode');
  const registrationEl = document.getElementById('mcpRegistration');
  const authMode = authModeEl ? authModeEl.value : 'no_auth';
  const registration = registrationEl ? registrationEl.value : 'cimd';
  const requestHeaders = collectMCPHeaders();

  if (!url || (!url.startsWith('http://') && !url.startsWith('https://'))) {
    msg.textContent = 'Vui lòng nhập URL MCP hợp lệ (bắt đầu bằng http:// hoặc https://)';
    msg.className = 'msg error';
    return;
  }

  const headerErr = validateMCPHeaders(requestHeaders, authMode);
  if (headerErr) {
    msg.textContent = headerErr;
    msg.className = 'msg error';
    return;
  }

  let clientId = '';
  let clientSecret = '';
  if (authMode !== 'no_auth' && registration === 'static_per_installation') {
    const clientIdEl = document.getElementById('mcpClientId');
    const clientSecretEl = document.getElementById('mcpClientSecret');
    clientId = (clientIdEl.value || '').trim();
    clientSecret = (clientSecretEl.value || '').trim();
    if (!clientId) {
      msg.textContent = 'Vui lòng nhập OAuth client ID khi dùng OAuth client riêng';
      msg.className = 'msg error';
      return;
    }
  }

  const authLabels = { auth_required: 'Đăng nhập ngay', defer_auth: 'Đăng nhập khi cần', no_auth: 'Không đăng nhập' };
  const regLabels = { cimd: 'CIMD', dcr: 'DCR', static_per_installation: 'OAuth client riêng' };
  let confirmText = 'Bạn có chắc muốn xóa tất cả MCP cũ và đồng bộ MCP mới cho TOÀN BỘ tài khoản trong pool không?\n\n' +
    'Tên: ' + name + '\nURL: ' + url + '\nXác thực: ' + (authLabels[authMode] || authMode);
  if (requestHeaders.length) {
    confirmText += '\nRequest headers: ' + requestHeaders.length;
  }
  if (authMode !== 'no_auth') {
    confirmText += '\nOAuth client: ' + (regLabels[registration] || registration);
    if (registration === 'static_per_installation') {
      confirmText += clientSecret ? ' (có client secret)' : ' (không có client secret)';
    }
    confirmText += '\n\nConnector OAuth có thể cần đăng nhập trên claude.ai sau khi tạo.';
  }
  if (authMode === 'no_auth' && requestHeaders.length) {
    confirmText += '\n\nRequest header sẽ được gửi dưới dạng static headers.';
  }

  if (!confirm(confirmText)) {
    return;
  }

  setMCPBusy(true);
  msg.textContent = 'Đang đồng bộ MCP cho các tài khoản... Vui lòng đợi trong giây lát.';
  msg.className = 'msg';
  resDiv.innerHTML = '';

  try {
    const payload = { name: name, url: url, authMode: authMode, headers: requestHeaders };
    if (authMode !== 'no_auth') {
      payload.registration = registration;
      if (registration === 'static_per_installation') {
        payload.clientId = clientId;
        payload.clientSecret = clientSecret;
      }
    }
    const res = await fetch('/keys/api/mcp/sync', {
      method: 'POST', headers,
      body: JSON.stringify(payload)
    });
    const data = await res.json();
    if (!res.ok) {
      msg.textContent = data.message || data.error || 'Đồng bộ thất bại';
      msg.className = 'msg error';
      return;
    }
    msg.textContent = data.message || 'Đồng bộ hoàn tất';
    msg.className = 'msg success';
    renderSyncResults(data.results);
  } catch (e) {
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    setMCPBusy(false);
  }
}

async function deleteMCP() {
  const msg = document.getElementById('mcpSyncMsg');
  const resDiv = document.getElementById('mcpSyncResults');
  const accountCount = document.querySelectorAll('#keysBody tr').length;
  const countText = accountCount > 0 ? accountCount + ' tài khoản' : 'TOÀN BỘ tài khoản';

  if (!confirm('Bạn có chắc muốn xóa TOÀN BỘ remote MCP custom của ' + countText + ' trong pool không? Thao tác này không thể hoàn tác.')) {
    return;
  }

  setMCPBusy(true);
  msg.textContent = 'Đang xóa MCP custom của các tài khoản... Vui lòng đợi trong giây lát.';
  msg.className = 'msg';
  resDiv.innerHTML = '';

  try {
    const res = await fetch('/keys/api/mcp/delete', {
      method: 'POST', headers,
      body: JSON.stringify({})
    });
    const data = await res.json();
    if (!res.ok) {
      msg.textContent = data.message || data.error || 'Xóa MCP thất bại';
      msg.className = 'msg error';
      return;
    }
    msg.textContent = data.message || 'Đã xóa MCP custom';
    msg.className = 'msg success';
    renderSyncResults(data.results);
  } catch (e) {
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    setMCPBusy(false);
  }
}

function setMCPBusy(busy) {
  ['mcpSyncBtn', 'mcpDeleteBtn', 'mcpAddHeaderBtn'].forEach(function(id) {
    const el = document.getElementById(id);
    if (!el) return;
    el.disabled = busy;
    el.style.opacity = busy ? '.5' : '';
  });
  if (!busy) updateMCPHeaderControls();
}

function renderSyncResults(results, containerId) {
  const resDiv = document.getElementById(containerId || 'mcpSyncResults');
  if (!results || !results.length) {
    resDiv.innerHTML = '';
    return;
  }
  let html = '<div style="background:var(--bg); border:1px solid var(--border); border-radius:8px; padding:.75rem; margin-top:.5rem;">';
  html += '<div style="font-weight:600; margin-bottom:.4rem; color:var(--text);">Chi tiết từng tài khoản:</div>';
  for (let i = 0; i < results.length; i++) {
    const r = results[i];
    let color = 'var(--err)';
    let tag = 'LỖI';
    let badgeClass = 'limited';
    if (r.status === 'ok') {
      color = 'var(--ok)';
      tag = 'THÀNH CÔNG';
      badgeClass = 'ok';
    } else if (r.status === 'warning') {
      color = 'var(--warn)';
      tag = 'CẢNH BÁO';
      badgeClass = 'limited';
    }
    html += '<div style="display:flex; justify-content:space-between; align-items:center; padding:.3rem 0; border-bottom:1px solid rgba(255,255,255,.05);">';
    html += '<div><code>' + escapeHtml(r.account) + '</code> <span style="color:var(--muted); font-size:.8rem; margin-left:.5rem;">' + escapeHtml(r.message) + '</span></div>';
    html += '<span class="badge ' + badgeClass + '" style="color:' + color + ';">' + tag + '</span>';
    html += '</div>';
  }
  html += '</div>';
  resDiv.innerHTML = html;
}

async function toggleAccount(cb, key) {
  const msg = document.getElementById('listMsg');
  cb.disabled = true;
  try {
    const res = await fetch('/keys/api/toggle', {
      method: 'POST', headers,
      body: JSON.stringify({ key, disabled: cb.checked })
    });
    if (!res.ok) {
      const data = await res.json();
      cb.checked = !cb.checked;
      msg.textContent = data.message || data.error || 'Lưu thất bại';
      msg.className = 'msg error';
      cb.disabled = false;
      return;
    }
    // re-render so the row reflects the new enabled/disabled look
    loadKeys();
  } catch (e) {
    cb.checked = !cb.checked;
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
    cb.disabled = false;
  }
}

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

async function unlockAccount(key) {
  const msg = document.getElementById('listMsg');
  const res = await fetch('/keys/api/unlock', {
    method: 'POST', headers,
    body: JSON.stringify({ key })
  });
  const data = await res.json();
  msg.textContent = data.message || data.error;
  msg.className = res.ok ? 'msg success' : 'msg error';
  delete checkResults[key];
  loadKeys();
}

async function loadRecentOnce() {
  try {
    const res = await fetch('/keys/api/recent', { headers });
    if (!res.ok) return;
    const data = await res.json();
    renderRecent(data.requests);
  } catch (e) { /* dashboard keeps working when the endpoint is unreachable */ }
  const es = new EventSource('/keys/api/recent/stream?apikey=' + encodeURIComponent(API_KEY));
  es.onmessage = function(ev) {
    try { renderRecent(JSON.parse(ev.data).requests); } catch (e) { /* ignore malformed push */ }
  };
}

function renderRecent(requests) {
    const tbody = document.getElementById('recentBody');
    const rows = (requests || []).map(function(r) {
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
        '<td style="text-align:right; white-space:nowrap; color:var(--muted);">' + fmt(r.chars) + '</td>' +
        '<td style="color:var(--muted);">' + escapeHtml(r.preview) + '</td></tr>';
    });
    tbody.innerHTML = rows.length
      ? rows.join('')
      : '<tr><td colspan="8" style="color:var(--muted);">Chưa có request nào</td></tr>';
}

// loadRecentOnce fetches the table a single time on page load, then opens an
// SSE stream that pushes a fresh list each time a chat request finishes. No
// timer, no polling: idle dashboard = zero requests to the server.

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
loadTools();
loadLastImage();
loadMCP();
initMCPForm();
loadPrivacy();
loadMemory();
loadRecentOnce();
setInterval(loadKeys, 3000);
setInterval(tickCountdowns, 1000);
</script>
</body>
</html>`

// KeysPageHandler serves the account/cookie management UI
func KeysPageHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(keysPageHTML))
}
