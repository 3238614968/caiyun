#!/data/data/com.termux/files/usr/bin/bash
# deploy/termux 脚本失败路径回归测试（作者复审要求：至少一条失败路径验证）
# 在 Termux 上运行：bash deploy/termux/test-failure-paths.sh
# 全部用例断言"脚本必须以非零退出码失败"；任何用例意外成功则整体 FAIL。
set -u
cd "$(dirname "$0")" || exit 1
rc=0

run_case() {
  local name=$1; shift
  if "$@" >/dev/null 2>&1; then
    echo "  [FAIL] $name: script exited 0 but should have failed"
    rc=1
  else
    echo "  [ok]   $name: failed as expected"
  fi
}

# 1) nginx：dist/index.html 缺失必须失败（P1 #3）
run_case "nginx missing dist" env CAIYUN_DIST=/nonexistent/dist-$$ bash ./deploy_nginx.sh

# 2) create_admin：管理员密码不满足后端策略（<8 位）必须失败
run_case "create_admin short password" env CAIYUN_ADMIN_USER=e2etest CAIYUN_ADMIN_PASS=short bash ./create_admin.sh

# 3) create_admin：用户名非法（SQL 注入样式）必须被白名单拒绝
run_case "create_admin bad username" env CAIYUN_ADMIN_USER="a'b\"c" CAIYUN_ADMIN_PASS=ValidPass123 bash ./create_admin.sh

# 4) deploy_infra：secrets 不可写（BASE 指向只读路径）必须失败
run_case "deploy_infra unwritable base" env HOME=/proc/self/nonexistent-$$ bash ./deploy_infra.sh

# 5) start_all：secrets.env 缺失必须失败
run_case "start_all missing secrets" env HOME=/tmp/empty-home-$$ bash ./start_all.sh

rm -rf /tmp/empty-home-$$ 2>/dev/null
if [ "$rc" -eq 0 ]; then
  echo "=== FAILURE-PATH TESTS PASSED ==="
else
  echo "=== FAILURE-PATH TESTS FAILED ==="
fi
exit $rc
