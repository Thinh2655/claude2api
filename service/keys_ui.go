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
  .container { max-width: 1040px; margin: 0 auto; }
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
  .mcp-header-row select.mcp-header-preset { width: auto; flex: 0 0 150px; margin-bottom: 0; }
  .mcp-header-row .mcp-header-value { flex: 2 1 180px; }
  .mcp-header-row button { padding: .4rem .6rem; }
  .mcp-required { display: flex; align-items: center; gap: .3rem; font-size: .8rem;
    color: var(--muted); white-space: nowrap; }
  .mcp-add-header { background: var(--bg); color: var(--text); border: 1px solid var(--border);
    margin-top: .5rem; }
  .mcp-add-header:disabled { opacity: .4; cursor: not-allowed; }
  .btn-secondary { background: var(--bg); color: var(--text); border: 1px solid var(--border); white-space: nowrap; }
  .btn-secondary:disabled { opacity: .4; cursor: not-allowed; }
  .badge.mode-allow { background: rgba(74,222,128,.15); color: var(--ok); }
  .badge.mode-ask { background: rgba(96,165,250,.15); color: #60a5fa; }
  .badge.mode-off { background: rgba(139,144,163,.15); color: var(--muted); }
  .mcp-server { margin-top: 1rem; }
  .mcp-tools-table td { padding: .5rem; vertical-align: top; }
  .mcp-tools-table select { width: auto; min-width: 13rem; margin-bottom: 0; }
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
  [hidden] { display: none !important; }
  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: .75rem; margin-bottom: 1rem; }
  .stat { background: var(--card); border: 1px solid var(--border); border-radius: 12px; padding: .8rem 1rem; min-width: 0; }
  .stat .n { font-size: 1.6rem; font-weight: 700; line-height: 1.15; }
  .stat .n.txt { font-size: 1rem; line-height: 1.9rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .stat .l { color: var(--muted); font-size: .78rem; margin-top: .15rem; }
  .stat.ok .n { color: var(--ok); }
  .stat.warn .n { color: var(--warn); }
  .stat.off .n { color: var(--muted); }
  .tabs { position: sticky; top: 0; z-index: 20; display: flex; gap: .25rem; background: var(--bg);
    padding: .5rem 0; margin-bottom: 1rem; border-bottom: 1px solid var(--border); overflow-x: auto; }
  .tab-btn { background: transparent; color: var(--muted); border-radius: 8px 8px 0 0; padding: .55rem 1.1rem;
    white-space: nowrap; font-size: .9rem; }
  .tab-btn:hover { opacity: 1; color: var(--text); }
  .tab-btn.active { background: var(--card); color: var(--text); box-shadow: inset 0 -2px 0 var(--accent); }
  .toolbar { display: flex; gap: .6rem; flex-wrap: wrap; align-items: center; margin-bottom: .75rem; }
  .toolbar input[type=text] { flex: 1 1 200px; margin-bottom: 0; font-family: inherit; }
  .toolbar select { width: auto; margin-bottom: 0; }
  .table-wrap { overflow-x: auto; }
  .empty-row td { text-align: center; color: var(--muted); padding: 1.5rem .5rem; }
  .badge.off { background: rgba(139,144,163,.15); color: var(--muted); }
  .badge.bad { background: rgba(248,113,113,.15); color: var(--err); }
  .toast-box { position: fixed; right: 1rem; bottom: 1rem; display: flex; flex-direction: column; gap: .5rem; z-index: 100; }
  .toast { background: var(--card); border: 1px solid var(--border); border-left: 4px solid var(--accent);
    padding: .65rem .9rem; border-radius: 8px; font-size: .85rem; max-width: 340px;
    box-shadow: 0 6px 20px rgba(0,0,0,.4); }
  .toast.success { border-left-color: var(--ok); }
  .toast.error { border-left-color: var(--err); }
  button:disabled { opacity: .5; cursor: not-allowed; }
  @media (max-width: 640px) {
    body { padding: 1rem; }
    .mcp-form-grid { flex-direction: column; align-items: stretch; }
    .switch-row { align-items: flex-start; }
  }
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

  <div class="stats">
    <div class="stat"><div class="n" id="statTotal">–</div><div class="l">Tổng tài khoản</div></div>
    <div class="stat ok"><div class="n" id="statReady">–</div><div class="l">Khả dụng</div></div>
    <div class="stat warn"><div class="n" id="statLimited">–</div><div class="l">Tạm khóa (limit)</div></div>
    <div class="stat off"><div class="n" id="statDisabled">–</div><div class="l">Đã tắt</div></div>
    <div class="stat"><div class="n" id="statTokens">–</div><div class="l">Token đã dùng</div><div class="l" id="statTokensSub" style="font-size:.72rem;"></div></div>
  </div>

  <div class="tabs" role="tablist">
    <button type="button" class="tab-btn active" data-tab="accounts" onclick="showTab('accounts')">Tài khoản</button>
    <button type="button" class="tab-btn" data-tab="settings" onclick="showTab('settings')">Cài đặt</button>
    <button type="button" class="tab-btn" data-tab="mcp" onclick="showTab('mcp')">MCP</button>
  </div>

  <div class="card" data-tab="accounts">
    <h2>Tài khoản</h2>
    <div class="toolbar">
      <input type="text" id="accSearch" placeholder="Tìm theo tên tài khoản..." autocomplete="off" oninput="onAccFilterChange()">
      <select id="accState" onchange="onAccFilterChange()">
        <option value="all">Tất cả trạng thái</option>
        <option value="ready">Khả dụng</option>
        <option value="cooling">Đang nghỉ</option>
        <option value="limited">Tạm khóa (limit)</option>
        <option value="disabled">Đã tắt</option>
      </select>
      <button id="checkBtn" onclick="checkKeys('')" style="background:rgba(217,119,87,.15); color:var(--accent);">Kiểm tra tất cả (claude.ai)</button>
    </div>
    <div class="hint" id="checkStatus" style="margin:0 0 .5rem;"></div>
    <div class="table-wrap">
    <table id="keysTable">
      <thead><tr><th>Tên acc</th><th>Trạng thái</th><th style="text-align:center;" title="Tài khoản dùng cho giao diện claude.ai tại cổng local">Gateway</th><th style="text-align:center;" title="Bật = tài khoản tham gia xoay vòng request">Kích hoạt</th><th></th></tr></thead>
      <tbody id="keysBody"></tbody>
    </table>
    </div>
    <div style="margin-top:.75rem; display:flex; gap:.75rem; align-items:center; flex-wrap:wrap;">
      <button class="btn-secondary" id="gwClearBtn" onclick="saveGatewayKey('')" title="Để hệ thống tự chọn tài khoản có cookie">Gateway: tự động chọn</button>
    </div>
    <div class="msg" id="listMsg"></div>
    <div class="hint" style="margin-top:.5rem;">Gateway: chọn radio để cố định tài khoản dùng cho giao diện claude.ai tại cổng local, hoặc bấm "Gateway: tự động chọn" để hệ thống tự chọn. Kích hoạt: tắt công tắc để loại tài khoản khỏi vòng xoay request (không xóa).</div>
  </div>

  <div class="card" data-tab="settings">
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

  <div class="card" data-tab="mcp">
    <h2>Đồng bộ MCP</h2>
    <div class="mcp-form-grid">
      <div>
        <label class="mcp-field-label" for="mcpSyncName">Tên MCP</label>
        <input type="text" id="mcpSyncName" placeholder="Tên MCP (vd: agent-mcp)" autocomplete="off" value="agent-mcp">
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
    <div class="mcp-field">
      <label class="mcp-field-label" for="mcpToolMode">Quyền tool mặc định</label>
      <select id="mcpToolMode" onchange="onMCPDefaultChange(this.value)">
        <option value="always_allow" selected>Always allow - tự chạy, không hỏi</option>
        <option value="use_tool">Use tool - hỏi duyệt mỗi lần gọi</option>
      </select>
      <div class="hint">Áp cho mọi tool của MCP khi đồng bộ; ghi đè riêng từng tool ở mục Quyền tool MCP bên dưới. Use tool chỉ duyệt được trên giao diện claude.ai (gateway); gọi qua API không có chỗ duyệt nên request sẽ bị dừng với lỗi cần duyệt.</div>
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
    <div class="hint">Đồng bộ: xóa tất cả remote MCP custom cũ của từng tài khoản trong pool và thêm MCP mới với cấu hình xác thực/header bên trên (quyền tool theo ô Quyền tool mặc định và các ghi đè riêng từng tool). Connector dùng OAuth có thể cần đăng nhập trên claude.ai sau khi tạo.<br>OAuth client ID/secret và giá trị header chỉ dùng cho lần đồng bộ này, không lưu vào .env.<br>Xóa MCP custom: xóa toàn bộ remote MCP custom của tất cả tài khoản, không thêm mới và không thể hoàn tác.</div>
    <div id="mcpSyncResults" style="margin-top:.75rem; font-size:.85rem;"></div>
  </div>

  <div class="card" data-tab="mcp">
    <h2>Quyền tool MCP</h2>
    <div class="mcp-field">
      <label class="mcp-field-label" for="mcpToolModePerm">Quyền mặc định</label>
      <select id="mcpToolModePerm" onchange="onMCPDefaultChange(this.value)">
        <option value="always_allow" selected>Always allow - tự chạy, không hỏi</option>
        <option value="use_tool">Use tool - hỏi duyệt mỗi lần gọi</option>
      </select>
    </div>
    <div style="display:flex; gap:.75rem; align-items:stretch;">
      <button class="btn-secondary" id="mcpToolsLoadBtn" onclick="loadMCPTools()">Tải danh sách tool</button>
      <button class="btn-primary" id="mcpPermApplyBtn" onclick="applyMCPPermissions()" style="flex:1;">Lưu và áp dụng cho tất cả tài khoản</button>
    </div>
    <div class="msg" id="mcpPermMsg"></div>
    <div class="hint">Tải danh sách tool từ tài khoản đầu tiên đang bật, rồi chọn quyền riêng cho từng tool (để Theo mặc định = dùng quyền mặc định). Bấm Lưu và áp dụng để ghi vào mọi tài khoản trong pool, không xóa hay tạo lại connector. Quyền được lưu vào MCP_TOOL_MODE / MCP_TOOL_MODES và dùng cho các lần đồng bộ sau. Ghi đè theo tên tool, nên tool trùng tên ở nhiều MCP dùng chung một quyền.</div>
    <div id="mcpToolsList"></div>
    <div id="mcpPermResults" style="margin-top:.75rem; font-size:.85rem;"></div>
  </div>

  <div class="card" data-tab="settings">
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

  <div class="card" data-tab="settings">
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

  <div class="card" data-tab="accounts">
    <h2>Thêm tài khoản (dán cookie)</h2>
    <textarea id="newCookie" placeholder='Dán JSON mảng cookie (EditThisCookie/Cookie-Editor export) tại đây' autocomplete="off"></textarea>
    <button class="btn-primary" id="addBtn" onclick="addAccount()">Thêm vào pool</button>
    <div class="msg" id="addMsg"></div>
    <div class="hint">Cookie được lưu vào <code>accounts.json</code> và áp dụng tức thì. Tên acc tự lấy từ claude.ai.</div>
  </div>

  <div class="card" data-tab="accounts">
    <h2>5 request mới nhất</h2>
    <div class="table-wrap">
    <table id="recentTable">
      <thead><tr><th>Thời gian</th><th>Tài khoản</th><th>Model</th><th style="text-align:right;">In ↑</th><th style="text-align:right;">Out ↓</th><th>Kết quả</th><th style="text-align:right;">Ký tự</th><th>Nội dung</th></tr></thead>
      <tbody id="recentBody">
        <tr><td colspan="8" style="color:var(--muted);">Chưa có request nào</td></tr>
      </tbody>
    </table>
    </div>
  </div>
</div>
<div class="toast-box" id="toastBox" aria-live="polite"></div>

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
  if (k.disabled) return '<span class="badge off" title="Tài khoản đã bị tắt, không nhận request">Đã tắt</span>';
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

let lastKeys = [];
let lastKeysSig = '';
let gatewayLoaded = false;
const accFilter = { q: '', state: 'all' };

function keyState(k) {
  if (k.disabled) return 'disabled';
  if (k.limited) return 'limited';
  if (k.cooling) return 'cooling';
  return 'ready';
}

function toast(text, type) {
  const box = document.getElementById('toastBox');
  if (!box) return;
  const el = document.createElement('div');
  el.className = 'toast' + (type ? ' ' + type : '');
  el.textContent = text;
  box.appendChild(el);
  setTimeout(function() { el.remove(); }, type === 'error' ? 6000 : 3500);
}

function onAccFilterChange() {
  accFilter.q = (document.getElementById('accSearch').value || '').trim().toLowerCase();
  accFilter.state = document.getElementById('accState').value;
  renderKeys(true);
}

function showTab(name) {
  document.querySelectorAll('.tab-btn').forEach(function(b) {
    b.classList.toggle('active', b.getAttribute('data-tab') === name);
  });
  document.querySelectorAll('.card[data-tab]').forEach(function(c) {
    c.hidden = c.getAttribute('data-tab') !== name;
  });
  try { localStorage.setItem('c2a_tab', name); } catch (e) { /* storage blocked */ }
}

function updateStats() {
  let limited = 0, disabled = 0, ready = 0;
  lastKeys.forEach(function(k) {
    if (k.disabled) disabled++;
    else if (k.limited) limited++;
    else ready++;
  });
  document.getElementById('statTotal').textContent = lastKeys.length;
  document.getElementById('statReady').textContent = ready;
  document.getElementById('statLimited').textContent = limited;
  document.getElementById('statDisabled').textContent = disabled;
  document.getElementById('gwClearBtn').disabled = !gatewayKey;
}

// Compact number for the stat card: 950, 12.3K, 4.56M.
function fmtCompact(n) {
  n = Number(n) || 0;
  if (n >= 1e9) return (n / 1e9).toFixed(2).replace(/\.?0+$/, '') + 'B';
  if (n >= 1e6) return (n / 1e6).toFixed(2).replace(/\.?0+$/, '') + 'M';
  if (n >= 1e4) return (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'K';
  return fmt(n);
}

// Totals are counted by the server since it started (in-memory, reset on restart).
function updateTokenStat(totals) {
  const nEl = document.getElementById('statTokens');
  const subEl = document.getElementById('statTokensSub');
  if (!nEl || !totals) return;
  const p = totals.promptTokens || 0, c = totals.completionTokens || 0;
  nEl.textContent = fmtCompact(p + c);
  nEl.title = fmt(p + c) + ' token';
  subEl.textContent = '↑ ' + fmtCompact(p) + ' · ↓ ' + fmtCompact(c) + ' · ' + fmt(totals.requests || 0) + ' req';
  subEl.title = 'Tính từ lúc server khởi động (về 0 khi khởi động lại)';
}

function renderKeys(force) {
  updateStats();
  const filtered = lastKeys.filter(function(k) {
    if (accFilter.state !== 'all' && keyState(k) !== accFilter.state) return false;
    if (accFilter.q && label(k).toLowerCase().indexOf(accFilter.q) === -1) return false;
    return true;
  });
  // Skip the DOM rebuild when nothing visible changed (countdowns tick on their own).
  const sig = JSON.stringify([gatewayKey, accFilter, filtered.map(function(k) {
    const cr = checkResults[k.full];
    return [k.full, k.displayName, k.disabled, k.limited, k.limitedUntil, k.cooling, cr ? cr.status : ''];
  })]);
  if (!force && sig === lastKeysSig) return;
  lastKeysSig = sig;

  const tbody = document.getElementById('keysBody');
  if (!filtered.length) {
    tbody.innerHTML = '<tr class="empty-row"><td colspan="5">' +
      (lastKeys.length ? 'Không có tài khoản nào khớp bộ lọc' : 'Chưa có tài khoản nào — hãy dán cookie ở mục "Thêm tài khoản" bên dưới') +
      '</td></tr>';
    return;
  }
  tbody.innerHTML = filtered.map(function(k) {
    const key = escapeHtml(k.full);
    const locked = !!k.limited;
    const unusable = k.disabled || locked;
    const isGw = k.full === gatewayKey;
    let actions = '<button class="btn-secondary" style="margin-right:.4rem;" data-act="check" data-key="' + key + '">Kiểm tra</button>';
    if (locked) {
      actions += '<button style="margin-right:.4rem; background:rgba(74,222,128,.15); color:var(--ok);" data-act="unlock" data-key="' + key + '">Mở khóa</button>';
    }
    actions += '<button class="btn-danger" data-act="delete" data-key="' + key + '">Xóa</button>';
    let notes = '';
    if (locked && k.limitedUntil) notes += '<div class="hint" style="margin-top:.2rem;">mở lại lúc ' + escapeHtml(k.limitedUntil) + '</div>';
    if (isGw && unusable) notes += '<div class="hint" style="margin-top:.2rem; color:var(--warn);">Đang là gateway nhưng hiện không dùng được</div>';
    return '<tr>' +
      '<td><code' + (k.disabled ? ' style="opacity:.45; text-decoration:line-through;"' : '') + '>' + escapeHtml(label(k)) + '</code>' + notes + '</td>' +
      '<td>' + statusBadge(k) + '</td>' +
      '<td style="text-align:center;"><input type="radio" name="gw" data-key="' + key + '"' + (isGw ? ' checked' : '') + (unusable ? ' disabled' : '') + '></td>' +
      '<td style="text-align:center;"><label class="switch" title="' + (k.disabled ? 'Đang tắt — bấm để bật' : 'Đang bật — bấm để tắt') + '"><input type="checkbox" class="acc-toggle" data-key="' + key + '"' + (k.disabled ? '' : ' checked') + '><span class="slider"></span></label></td>' +
      '<td style="text-align:right; white-space:nowrap;">' + actions + '</td></tr>';
  }).join('');
}

async function loadKeys() {
  let res;
  try {
    res = await fetch('/keys/api', { headers });
  } catch (e) {
    return; // transient network error: keep the last render, retry on next poll
  }
  const listMsg = document.getElementById('listMsg');
  if (res.status === 401) {
    listMsg.textContent = 'Thiếu API key — mở trang dạng /keys?apikey=<APIKEY của bạn>';
    listMsg.className = 'msg error';
    return;
  }
  if (listMsg.className === 'msg error' && listMsg.textContent.indexOf('Thiếu API key') === 0) {
    listMsg.textContent = '';
    listMsg.className = 'msg';
  }
  const data = await res.json();
  lastKeys = data.keys || [];

  // read the gateway selection from the server once, before the first render
  if (!gatewayLoaded) {
    try {
      const gwRes = await fetch('/keys/api/gateway', { headers });
      if (gwRes.ok) {
        const gwData = await gwRes.json();
        gatewayKey = gwData.gatewayKey || '';
      }
      gatewayLoaded = true;
    } catch (e) { /* retry on next poll */ }
  }
  renderKeys(false);
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

const MCP_MODE_LABELS = { always_allow: 'Always allow', use_tool: 'Use tool', disabled: 'Tắt', unset: 'Chưa thiết lập' };
// Per-tool overrides as stored on the server (tool name -> mode). mcpPermsReady
// guards against sending an empty override map before the stored one was read.
let mcpToolOverrides = {};
let mcpPermsReady = false;
let mcpToolsShown = false;

async function loadMCP() {
  const res = await fetch('/keys/api/mcp', { headers });
  if (!res.ok) return;
  const data = await res.json();
  document.getElementById('mcpToggle').checked = !!data.enableMCP;
  mcpToolOverrides = data.toolModes || {};
  mcpPermsReady = true;
  onMCPDefaultChange(data.toolMode || 'always_allow');
}

// Keeps the two default-mode selects (sync card + permission card) in step.
function onMCPDefaultChange(value) {
  ['mcpToolMode', 'mcpToolModePerm'].forEach(function(id) {
    const el = document.getElementById(id);
    if (el && el.value !== value) el.value = value;
  });
  refreshMCPDefaultLabels();
}

function refreshMCPDefaultLabels() {
  const el = document.getElementById('mcpToolModePerm');
  const def = el ? el.value : 'always_allow';
  document.querySelectorAll('#mcpToolsList .mcp-tool-mode option[value=""]').forEach(function(o) {
    o.textContent = 'Theo mặc định (' + (MCP_MODE_LABELS[def] || def) + ')';
  });
}

function mcpModeBadge(mode) {
  let cls = 'mode-off';
  if (mode === 'always_allow') cls = 'mode-allow';
  else if (mode === 'use_tool') cls = 'mode-ask';
  return '<span class="badge ' + cls + '">' + escapeHtml(MCP_MODE_LABELS[mode] || mode) + '</span>';
}

function mcpModeOptions(selected) {
  const opts = [['', 'Theo mặc định'], ['always_allow', 'Always allow'], ['use_tool', 'Use tool']];
  return opts.map(function(o) {
    return '<option value="' + o[0] + '"' + (o[0] === selected ? ' selected' : '') + '>' + o[1] + '</option>';
  }).join('');
}

function renderMCPTools(servers) {
  const box = document.getElementById('mcpToolsList');
  if (!servers.length) {
    box.innerHTML = '<div class="hint">Tài khoản này chưa có MCP custom nào. Hãy đồng bộ MCP trước.</div>';
    return;
  }
  let html = '';
  servers.forEach(function(s) {
    html += '<div class="mcp-server"><div class="mcp-section-title">' + escapeHtml(s.name) +
      ' <span class="hint">' + escapeHtml(s.url) + '</span></div>';
    const tools = s.tools || [];
    if (s.error) {
      html += '<div class="hint" style="color:var(--err);">Không tải được tool: ' + escapeHtml(s.error) + '</div>';
    } else if (!tools.length) {
      html += '<div class="hint">Server không có tool nào.</div>';
    } else {
      html += '<table class="mcp-tools-table"><tbody>';
      tools.forEach(function(t) {
        html += '<tr><td><code>' + escapeHtml(t.name) + '</code>' +
          (t.description ? '<div class="hint">' + escapeHtml(t.description) + '</div>' : '') + '</td>' +
          '<td style="white-space:nowrap;">' + mcpModeBadge(t.mode) + '</td>' +
          '<td><select class="mcp-tool-mode" data-tool="' + escapeHtml(t.name) + '">' +
          mcpModeOptions(mcpToolOverrides[t.name] || '') + '</select></td></tr>';
      });
      html += '</tbody></table>';
    }
    html += '</div>';
  });
  box.innerHTML = html;
  refreshMCPDefaultLabels();
}

// Stored overrides, updated with what the visible selects say. Tools that are
// not on screen (e.g. a server that failed to load) keep their stored mode.
function collectMCPToolModes() {
  const modes = Object.assign({}, mcpToolOverrides);
  document.querySelectorAll('#mcpToolsList .mcp-tool-mode').forEach(function(sel) {
    const tool = sel.getAttribute('data-tool');
    if (sel.value) modes[tool] = sel.value;
    else delete modes[tool];
  });
  return modes;
}

async function loadMCPTools(silent) {
  const msg = document.getElementById('mcpPermMsg');
  const box = document.getElementById('mcpToolsList');
  setMCPBusy(true);
  if (!silent) {
    msg.textContent = 'Đang tải danh sách tool từ claude.ai...';
    msg.className = 'msg';
  }
  try {
    const res = await fetch('/keys/api/mcp/tools', { headers });
    const data = await res.json();
    if (!res.ok) {
      mcpToolsShown = false;
      box.innerHTML = '';
      msg.textContent = data.message || data.error || 'Tải danh sách tool thất bại';
      msg.className = 'msg error';
      return;
    }
    mcpToolOverrides = data.toolModes || {};
    mcpPermsReady = true;
    onMCPDefaultChange(data.toolMode || 'always_allow');
    renderMCPTools(data.servers || []);
    mcpToolsShown = true;
    if (!silent) {
      msg.textContent = 'Đã tải từ tài khoản ' + data.account;
      msg.className = 'msg success';
    }
  } catch (e) {
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    setMCPBusy(false);
  }
}

async function applyMCPPermissions() {
  const msg = document.getElementById('mcpPermMsg');
  const resDiv = document.getElementById('mcpPermResults');
  const mode = document.getElementById('mcpToolModePerm').value;
  const payload = { toolMode: mode };
  if (mcpPermsReady) payload.toolModes = collectMCPToolModes();
  const overrides = payload.toolModes ? Object.keys(payload.toolModes).length : 0;

  let confirmText = 'Áp dụng quyền tool MCP cho TOÀN BỘ tài khoản trong pool?\n\nMặc định: ' + (MCP_MODE_LABELS[mode] || mode);
  if (overrides) confirmText += '\nGhi đè riêng: ' + overrides + ' tool';
  if (mode === 'use_tool' || Object.keys(payload.toolModes || {}).some(function(k) { return payload.toolModes[k] === 'use_tool'; })) {
    confirmText += '\n\nTool để Use tool sẽ yêu cầu duyệt trên claude.ai; gọi qua API sẽ bị dừng với lỗi cần duyệt.';
  }
  if (!confirm(confirmText)) return;

  setMCPBusy(true);
  msg.textContent = 'Đang áp dụng quyền tool cho các tài khoản... Vui lòng đợi trong giây lát.';
  msg.className = 'msg';
  resDiv.innerHTML = '';
  let applied = false;
  try {
    const res = await fetch('/keys/api/mcp/permissions', {
      method: 'POST', headers,
      body: JSON.stringify(payload)
    });
    const data = await res.json();
    if (!res.ok) {
      msg.textContent = data.message || data.error || 'Áp dụng quyền tool thất bại';
      msg.className = 'msg error';
      return;
    }
    mcpToolOverrides = data.toolModes || {};
    msg.textContent = data.message || 'Đã áp dụng quyền tool';
    msg.className = 'msg success';
    renderSyncResults(data.results, 'mcpPermResults');
    applied = true;
  } catch (e) {
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    setMCPBusy(false);
  }
  // Refresh the badges so they show what claude.ai now reports.
  if (applied && mcpToolsShown) await loadMCPTools(true);
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
  updateMCPPresetState();
  if (mode === 'auth_required') {
    hint.textContent = 'Mỗi người dùng phải đăng nhập qua OAuth của server trước khi dùng tool.';
  } else if (mode === 'defer_auth') {
    hint.textContent = 'Claude kết nối trước, chỉ yêu cầu đăng nhập khi server cần. Phù hợp server có tool công khai.';
  } else {
    hint.textContent = 'Không dùng OAuth. Nếu khai báo request header, connector sẽ dùng static headers (ví dụ API key).';
  }
}

const MCP_HEADER_DEFAULT = 'x-api-key';
const MCP_HEADER_PRESETS = [
  'authorization', 'x-api-key', 'api-key', 'apikey', 'x-apikey', 'x-api-token', 'api-token',
  'x-auth-token', 'x-access-token', 'x-token', 'x-key', 'access-key', 'x-api-secret',
  'x-secret-key', 'x-app-key', 'x-app-id', 'x-application-key', 'x-application-id',
  'x-client-key', 'x-client-id', 'x-client-secret', 'app-key', 'ocp-apim-subscription-key',
  'x-functions-key', 'x-gateway-api-key', 'x-gateway-key', 'x-rapidapi-key', 'x-rapidapi-host',
  'x-tenant-id', 'x-tenant', 'x-tenant-key', 'x-org-id', 'x-org', 'x-organization',
  'x-organization-id', 'x-workspace-id', 'x-workspace', 'x-account-id', 'x-account-key',
  'x-project-id', 'x-project-key', 'x-environment', 'x-cost-center', 'x-department',
  'x-business-unit'
];

function mcpAuthIsOAuth() {
  const modeEl = document.getElementById('mcpAuthMode');
  return !!modeEl && modeEl.value !== 'no_auth';
}

// Authorization do OAuth quản lý nên bị vô hiệu hóa khi không ở chế độ "Không đăng nhập".
function updateMCPPresetState() {
  const oauth = mcpAuthIsOAuth();
  document.querySelectorAll('#mcpHeaderRows .mcp-header-preset option[value="authorization"]').forEach(function(opt) {
    opt.disabled = oauth;
  });
}

function buildMCPHeaderPreset(nameInput) {
  const select = document.createElement('select');
  select.className = 'mcp-header-preset';
  select.title = 'Chọn nhanh tên header';

  const placeholder = document.createElement('option');
  placeholder.value = '';
  placeholder.textContent = 'Chọn nhanh…';
  select.appendChild(placeholder);

  MCP_HEADER_PRESETS.forEach(function(h) {
    const opt = document.createElement('option');
    opt.value = h;
    opt.textContent = h;
    select.appendChild(opt);
  });

  const custom = document.createElement('option');
  custom.value = '__custom__';
  custom.textContent = 'Tiêu đề tùy chỉnh…';
  select.appendChild(custom);

  select.onchange = function() {
    if (select.value === '__custom__') {
      nameInput.value = '';
      nameInput.focus();
    } else if (select.value) {
      nameInput.value = select.value;
      const row = nameInput.closest('.mcp-header-row');
      if (row) delete row.dataset.defaultRow;
    }
    select.value = '';
  };
  return select;
}

function addMCPHeader(name, value, required, isDefault) {
  const rows = document.getElementById('mcpHeaderRows');
  if (!rows) return;
  if (rows.children.length >= 4) return;

  const row = document.createElement('div');
  row.className = 'mcp-header-row';
  // Dòng mặc định (x-api-key) còn nguyên và chưa có value thì bị bỏ qua khi đồng bộ.
  if (isDefault) row.dataset.defaultRow = '1';

  const nameInput = document.createElement('input');
  nameInput.type = 'text';
  nameInput.className = 'mcp-header-name';
  nameInput.placeholder = 'Tên header (vd: x-api-key)';
  nameInput.maxLength = 64;
  nameInput.autocomplete = 'off';
  nameInput.value = name || '';
  nameInput.addEventListener('input', function() { delete row.dataset.defaultRow; });

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
  row.appendChild(buildMCPHeaderPreset(nameInput));
  row.appendChild(valueInput);
  row.appendChild(requiredLabel);
  row.appendChild(removeButton);
  rows.appendChild(row);
  updateMCPPresetState();
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
    if (row.dataset.defaultRow === '1' && !value) return;
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
  if (rows && rows.children.length === 0) addMCPHeader(MCP_HEADER_DEFAULT, '', true, true);
}

async function syncMCP() {
  const nameInput = document.getElementById('mcpSyncName');
  const urlInput = document.getElementById('mcpSyncUrl');
  const name = (nameInput.value || '').trim() || 'agent-mcp';
  const url = (urlInput.value || '').trim();
  const msg = document.getElementById('mcpSyncMsg');
  const resDiv = document.getElementById('mcpSyncResults');
  const authModeEl = document.getElementById('mcpAuthMode');
  const registrationEl = document.getElementById('mcpRegistration');
  const authMode = authModeEl ? authModeEl.value : 'no_auth';
  const registration = registrationEl ? registrationEl.value : 'cimd';
  const requestHeaders = collectMCPHeaders();
  const toolModeEl = document.getElementById('mcpToolMode');
  const toolMode = toolModeEl ? toolModeEl.value : 'always_allow';

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
    'Tên: ' + name + '\nURL: ' + url + '\nXác thực: ' + (authLabels[authMode] || authMode) +
    '\nQuyền tool mặc định: ' + (MCP_MODE_LABELS[toolMode] || toolMode);
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
    const payload = { name: name, url: url, authMode: authMode, headers: requestHeaders, toolMode: toolMode };
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
  ['mcpSyncBtn', 'mcpDeleteBtn', 'mcpAddHeaderBtn', 'mcpToolsLoadBtn', 'mcpPermApplyBtn'].forEach(function(id) {
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
  const enable = cb.checked;
  cb.disabled = true;
  try {
    const res = await fetch('/keys/api/toggle', {
      method: 'POST', headers,
      body: JSON.stringify({ key, disabled: !enable })
    });
    const data = await res.json().catch(function() { return {}; });
    if (!res.ok) {
      cb.checked = !enable;
      cb.disabled = false;
      toast(data.message || data.error || 'Lưu thất bại', 'error');
      return;
    }
    toast(enable ? 'Đã bật tài khoản' : 'Đã tắt tài khoản — sẽ không nhận request', 'success');
    // re-render so the row reflects the new enabled/disabled look
    loadKeys();
  } catch (e) {
    cb.checked = !enable;
    cb.disabled = false;
    toast('Lỗi kết nối: ' + e.message, 'error');
  }
}

function fmt(n) { return String(n || 0).replace(/\B(?=(\d{3})+(?!\d))/g, ','); }

async function saveGatewayKey(key) {
  const prev = gatewayKey;
  gatewayKey = key || '';
  try {
    const res = await fetch('/keys/api/gateway', {
      method: 'POST', headers,
      body: JSON.stringify({ key: gatewayKey })
    });
    const data = await res.json().catch(function() { return {}; });
    if (!res.ok) {
      gatewayKey = prev;
      toast(data.message || data.error || 'Lưu gateway thất bại', 'error');
    } else {
      toast(data.message || (gatewayKey ? 'Đã đặt tài khoản gateway' : 'Gateway sẽ tự chọn tài khoản'), 'success');
    }
  } catch (e) {
    gatewayKey = prev;
    toast('Lỗi kết nối: ' + e.message, 'error');
  }
  renderKeys(true);
}

function escapeHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

async function addAccount() {
  const cookie = document.getElementById('newCookie').value.trim();
  const msg = document.getElementById('addMsg');
  const btn = document.getElementById('addBtn');
  if (!cookie) { msg.textContent = 'Dán JSON cookie trước'; msg.className = 'msg error'; return; }
  btn.disabled = true;
  msg.textContent = 'Đang kiểm tra cookie và thêm vào pool...';
  msg.className = 'msg';
  try {
    const res = await fetch('/keys/api/add', {
      method: 'POST', headers,
      body: JSON.stringify({ cookie })
    });
    const data = await res.json().catch(function() { return {}; });
    msg.textContent = data.message || data.error || (res.ok ? 'Đã thêm' : 'Thêm thất bại');
    msg.className = res.ok ? 'msg success' : 'msg error';
    if (res.ok) {
      document.getElementById('newCookie').value = '';
      toast('Đã thêm tài khoản vào pool', 'success');
    }
  } catch (e) {
    msg.textContent = 'Lỗi kết nối: ' + e.message;
    msg.className = 'msg error';
  } finally {
    btn.disabled = false;
  }
  loadKeys();
}

async function deleteAccount(key) {
  const found = lastKeys.find(function(k) { return k.full === key; });
  const name = found ? label(found) : 'tài khoản này';
  let warn = '';
  if (key === gatewayKey) warn = '\n\nTài khoản này đang là gateway claude.ai.';
  if (!confirm('Xoá "' + name + '" khỏi pool? Thao tác không thể hoàn tác.' + warn)) return;
  try {
    const res = await fetch('/keys/api/delete', {
      method: 'POST', headers,
      body: JSON.stringify({ key })
    });
    const data = await res.json().catch(function() { return {}; });
    toast(data.message || data.error || (res.ok ? 'Đã xóa' : 'Xóa thất bại'), res.ok ? 'success' : 'error');
    if (res.ok && key === gatewayKey) gatewayKey = '';
  } catch (e) {
    toast('Lỗi kết nối: ' + e.message, 'error');
  }
  delete checkResults[key];
  loadKeys();
}

async function unlockAccount(key) {
  try {
    const res = await fetch('/keys/api/unlock', {
      method: 'POST', headers,
      body: JSON.stringify({ key })
    });
    const data = await res.json().catch(function() { return {}; });
    toast(data.message || data.error || (res.ok ? 'Đã mở khóa' : 'Mở khóa thất bại'), res.ok ? 'success' : 'error');
  } catch (e) {
    toast('Lỗi kết nối: ' + e.message, 'error');
  }
  delete checkResults[key];
  loadKeys();
}

async function loadRecentOnce() {
  try {
    const res = await fetch('/keys/api/recent', { headers });
    if (!res.ok) return;
    const data = await res.json();
    renderRecent(data.requests);
    updateTokenStat(data.totals);
  } catch (e) { /* dashboard keeps working when the endpoint is unreachable */ }
  const es = new EventSource('/keys/api/recent/stream?apikey=' + encodeURIComponent(API_KEY));
  es.onmessage = function(ev) {
    try {
      const d = JSON.parse(ev.data);
      renderRecent(d.requests);
      updateTokenStat(d.totals);
    } catch (e) { /* ignore malformed push */ }
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

function initKeysTable() {
  const tbody = document.getElementById('keysBody');
  tbody.addEventListener('click', function(ev) {
    const btn = ev.target.closest('button[data-act]');
    if (!btn) return;
    const key = btn.getAttribute('data-key');
    const act = btn.getAttribute('data-act');
    if (act === 'check') checkKeys(key);
    else if (act === 'unlock') unlockAccount(key);
    else if (act === 'delete') deleteAccount(key);
  });
  tbody.addEventListener('change', function(ev) {
    const el = ev.target;
    if (el.name === 'gw') saveGatewayKey(el.getAttribute('data-key'));
    else if (el.classList.contains('acc-toggle')) toggleAccount(el, el.getAttribute('data-key'));
  });
  let tab = 'accounts';
  try { tab = localStorage.getItem('c2a_tab') || tab; } catch (e) { /* storage blocked */ }
  if (tab !== 'accounts' && tab !== 'settings' && tab !== 'mcp') tab = 'accounts';
  showTab(tab);
}

initKeysTable();
loadKeys();
loadTools();
loadLastImage();
loadMCP();
initMCPForm();
loadPrivacy();
loadMemory();
loadRecentOnce();
// Poll only while the tab is visible; the table is rebuilt only when something changed.
setInterval(function() { if (!document.hidden) loadKeys(); }, 5000);
setInterval(tickCountdowns, 1000);
</script>
</body>
</html>`

// KeysPageHandler serves the account/cookie management UI
func KeysPageHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(keysPageHTML))
}
