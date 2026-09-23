# Kế hoạch: Chuyển claude2api sang dùng "account cookie file" thay vì sessionKey

## Mục tiêu
- Loại bỏ cách cũ: pool session dựa vào `SESSIONS` env (sessionKey thuần).
- Chỉ dùng **full browser cookie** (account cookie) — đã chứng minh không gặp lỗi limit 429.
- Sửa UI keys: thêm cookie (= dán JSON cookie file) thay vì thêm sessionKey; hiển thị tên acc (nhập tay tùy chọn, fallback masked sessionKey); giữ đúng phần còn lại UI.

## Phát hiện đã xác minh
- `cookie.json` (EditThisCookie/Cookie-Editor export) đã làm việc: khi có `__cf_bm`, `routingHint`, device-id → claude.ai trả 200 SSE (không 429). (Đã test proxy.)
- `GET /api/me`, `/api/session` → 404; `/api/organizations` chỉ trả org UUID, không có email. **Không lấy được email từ API** → tên acc cho user nhập tay (và đã được hỏi, user đồng ý nhập).
- Cookie module hiện tại (root `cookie.json`) trả 403 `account_session_invalid` → có thể hết hạn; cần thêm qua UI khi refresh.

## Thiết kế

### 1. Config (`config/config.go`)
- Thêm field `DisplayName string yaml:"displayName,omitempty"` vào `SessionInfo`.
- Thêm struct `CookieAccount { Name string; Cookies []CookieFile }` + hàm:
  - `LoadCookieAccounts()`: đọc `accounts.json` (preferred, array account) + `cookie.json` (legacy, 1 account) → gộp `[]SessionInfo`.
  - Giữ `LoadSessionFromCookie(cookiePath)` hiện tại làm cơ sở build 1 account (tái dùng).
- `loadConfigFromEnv()`: 
  - Load `SESSIONS` env (giữ như fallback back-compat), **nhưng** khi có `accounts.json`/`cookie.json` → thay pool = cookie accounts (bỏ sessionKey env).
  - Nếu không có bất kể account nào → giữ env sessions (để app không chết).

### 2. Core (`core/api.go`, `core/gateway.go`)
- Đã sẵn `NewClientWithCookie`, cookie header, device. **Giữ nguyên.**
- `gateway.pickSession`: ưu tiên account có cookie (đã làm). **Giữ.**

### 3. Service keys (`service/keys.go`)
- `keyInfo` thêm `DisplayName string json:"displayName,omitempty"`, `HasCookie bool`.
- `KeysListHandler`: trả cả `DisplayName` (+`Masked`).
- `KeyAddHandler`: body `{ "name":"...", "cookie":"<JSON array>" }` → parse, kiểm sessionKey trong array, tạo 1 account → thêm vào `accounts.json`. (persist qua `saveAccounts()`).
- `KeyDeleteHandler`: xóa theo key (sessionKey) hoặc theo index; persist lại.
- `saveSessionsToEnv` → `persistCookieAccounts()` ghi `accounts.json`.
- `GatewayKeyGetHandler/SetHandler`: không đổi hoạt động, nhưng `option` hiển thị DisplayName.

### 4. Key check (`service/keys_check.go`)
- Đổi: dùng `checkOneAccount` — build client từ cookie (NewClient), gọi `/api/organizations` (hoặc `/api/me`) với cookie → status valid/rate_limited.

### 5. Handle (`service/handle.go`)
- Log dùng `DisplayName` (nếu có) thay cho raw key để dễ đọc. Hành vi `SendMessageWithCreate` giữ cookie.

### 6. UI (`service/keys_ui.go`)
- Form "Thêm cookie": textarea JSON cookie + field "Tên hiển thị (tùy chọn)" + button Thêm.
- Bảng hiển thị cột **"Tên acc"** (`DisplayName` nếu có, else masked key) thay vì full cookie key.
- Gateway dropdown hiển thị `DisplayName`.
- Xóa dùng key (vẫn dựa sessionKey nội bộ) — giữ.
- Thay đổi các endpoint body cho khớp (add body {cookie, name}).

### 7. File storage
- `accounts.json` (gitignore). Khi thêm cookie mới → append object `{name, cookies}`. Khi xóa → bỏ.
- `cookie.json` legacy vẫn được đọc (1 account). Nếu user thêm cookie mới, lưu vào `accounts.json`.

## Sequence
1. Config: `DisplayName`, `CookieAccount`, `LoadCookieAccounts`, adjust `loadConfigFromEnv`.
2. Keys handlers: list/add/delete + persist accounts.
3. keys_check: account-based check.
4. handle: log hiển thị DisplayName.
5. UI: textarea cookie, tên acc, dropdown, table.
6. Build, build exe, test.

## Rủi ro / lưu ý
- `accounts.json`/`cookie.json` chứa secret — trong `.gitignore`.
- Không thay đổi auth local (APIKey) — vẫn giữ.
- Mirror API vẫn dùng sessionKey (không cookie): hàm `NewClient` (bare) giữ nguyên cho mirror.
- Cookie hết hạn: user phải re-export qua UI; display vẫn masked nếu không có name.