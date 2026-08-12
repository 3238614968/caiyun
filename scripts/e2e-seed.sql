-- Real Docker Compose E2E seed data.
-- Password for e2e-admin is: E2eAdminPass123!

INSERT INTO users (id, username, password, email, role, token_version, created_at, updated_at, deleted_at) VALUES
  (900001, 'e2e-admin', '$2a$10$A6MpkfmGCpT8h8wzTIMe/OBT0xxo28a5W1HIUumciQj2uWH6wH1Ne', 'e2e-admin@example.local', 'admin', 0, NOW(), NOW(), NULL)
ON DUPLICATE KEY UPDATE
  password = VALUES(password),
  email = VALUES(email),
  role = VALUES(role),
  token_version = 0,
  deleted_at = NULL,
  updated_at = NOW();

INSERT INTO accounts (id, user_id, phone, auth, token, jwt_token, platform, expire_at, cloud_count, remark, is_active, created_at, updated_at, deleted_at) VALUES
  (900001, 900001, '13900009001', 'enc:v1:AAECAwQFBgcICQoLSNfeUbhtpsSFrB3fEqq5a4psZvVjavutQYWWvHOUR+6zwNwB', 'enc:v1:EBESExQVFhcYGRobSD4TfnPzD4OtTk8yQeJ1a/9XZPsMSavsYQQh/eugcmU4vgsYOg', 'enc:v1:ICEiIyQlJicoKSory1viOrJgRpSEklu7Dk50eU9nxmKSCPdpXLue05PRi/mNHNI', 'pc', 4102415999000, 6666, 'E2E 主账号', TRUE, NOW(), NOW(), NULL)
ON DUPLICATE KEY UPDATE
  auth = VALUES(auth),
  token = VALUES(token),
  jwt_token = VALUES(jwt_token),
  cloud_count = VALUES(cloud_count),
  remark = VALUES(remark),
  is_active = TRUE,
  deleted_at = NULL,
  updated_at = NOW();

INSERT INTO cloud_stats (user_id, account_id, date, cloud_count, cloud_diff, cloud_diff_week, created_at, updated_at, deleted_at) VALUES
  (900001, 900001, CURRENT_DATE, 6666, 66, 166, NOW(), NOW(), NULL)
ON DUPLICATE KEY UPDATE
  cloud_count = VALUES(cloud_count),
  cloud_diff = VALUES(cloud_diff),
  cloud_diff_week = VALUES(cloud_diff_week),
  deleted_at = NULL,
  updated_at = NOW();

INSERT INTO task_logs (id, user_id, account_id, task_type, status, message, cloud_gained, execution_time, created_at, deleted_at) VALUES
  (900001, 900001, 900001, 'signin', 'success', 'E2E seeded sign-in log', 6, 120, NOW(), NULL),
  (900002, 900001, 900001, 'daily_cloud', 'success', 'E2E seeded daily cloud log', 60, 180, NOW(), NULL)
ON DUPLICATE KEY UPDATE
  status = VALUES(status),
  message = VALUES(message),
  cloud_gained = VALUES(cloud_gained),
  execution_time = VALUES(execution_time),
  deleted_at = NULL;

INSERT INTO products (id, prize_id, prize_name, p_order, category, daily_remainder_count, daily_limit_count, daily_count, image_url, stock_status, memo, is_active, is_deleted, updated_at, created_at, deleted_at) VALUES
  (900001, 'e2e-video-vip', 'E2E 视频会员', 1500, '视频类会员', 8, 1, 100, '', 'available', 'E2E product fixture', TRUE, FALSE, NOW(), NOW(), NULL),
  (900002, 'e2e-drink-coupon', 'E2E 喜茶兑换券', 900, '奶茶饮品权益', 0, 1, 50, '', 'sold_out', 'E2E sold out fixture', TRUE, FALSE, NOW(), NOW(), NULL)
ON DUPLICATE KEY UPDATE
  prize_name = VALUES(prize_name),
  p_order = VALUES(p_order),
  category = VALUES(category),
  daily_remainder_count = VALUES(daily_remainder_count),
  daily_limit_count = VALUES(daily_limit_count),
  daily_count = VALUES(daily_count),
  stock_status = VALUES(stock_status),
  memo = VALUES(memo),
  is_active = TRUE,
  is_deleted = FALSE,
  deleted_at = NULL,
  updated_at = NOW();

INSERT INTO announcements (id, title, content, is_popup, is_top, is_published, popup_count, created_at, updated_at) VALUES
  (900001, 'E2E 公告', '真实 Compose E2E 种子公告', TRUE, TRUE, TRUE, 0, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  title = VALUES(title),
  content = VALUES(content),
  is_popup = VALUES(is_popup),
  is_top = VALUES(is_top),
  is_published = VALUES(is_published),
  updated_at = NOW();

-- SSE replay fixture: the E2E check connects with Last-Event-ID=41 and must
-- receive this persisted sequence-42 message through the frontend proxy.
INSERT INTO web_socket_messages (user_id, message_id, sequence, type, data, is_read, is_delivered, created_at, expires_at) VALUES
  (900001, 'e2e-sse-42', 42, 'operation.updated', '{"operation_id":"e2e-seed","status":"succeeded"}', FALSE, FALSE, NOW(), NULL)
ON DUPLICATE KEY UPDATE
  sequence = VALUES(sequence),
  type = VALUES(type),
  data = VALUES(data),
  is_delivered = FALSE,
  delivered_at = NULL,
  expires_at = NULL;

