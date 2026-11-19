import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("proxy_config", ROOT / "scripts/configure_proxy_trust.py")
proxy_config = importlib.util.module_from_spec(spec)
spec.loader.exec_module(proxy_config)


class ProxyDeploymentTests(unittest.TestCase):
    def test_reject_missing_trust_all_and_injected_sources(self):
        for raw in ("", "none", "*", "0.0.0.0/0", "::/0", "0:0:0:0:0:0:0:0/0", "::ffff:0.0.0.0/96", "10.42.0.0/16\ninclude /tmp/evil.conf", "proxy.local"):
            with self.subTest(raw=raw), self.assertRaises(ValueError):
                proxy_config.validate_proxies(raw)

    def test_accept_explicit_ipv4_ipv6_and_deduplicate(self):
        self.assertEqual(proxy_config.validate_proxies("10.42.0.0/16, 10.42.1.9;fd00:42::/64 10.42.1.9"),
                         "10.42.0.0/16,10.42.1.9,fd00:42::/64")

    def test_base_manifest_requires_cluster_network_before_deploying(self):
        manifest = (ROOT / "k8s/caiyun.yaml").read_text(encoding="utf-8")
        with self.assertRaises(ValueError):
            proxy_config.render_manifest(manifest, None)
        rendered = proxy_config.render_manifest(manifest, "10.42.0.0/16")
        self.assertIn('  TRUSTED_PROXIES: "10.42.0.0/16"', rendered)
        self.assertIn('  REVERSE_PROXY_REQUIRED: "true"', rendered)
        self.assertIn("name: backend-api-ingress", rendered)
        self.assertIn("key: TRUSTED_PROXIES", rendered)
        self.assertEqual(proxy_config.render_manifest(rendered, None), rendered)

    def test_no_accidental_replacement_without_unique_config_entry(self):
        for manifest in ("data: {}", 'data:\n  TRUSTED_PROXIES: ""\n---\ndata:\n  TRUSTED_PROXIES: ""\n'):
            with self.subTest(manifest=manifest), self.assertRaises(ValueError):
                proxy_config.render_manifest(manifest, "10.42.0.0/16")

    def test_nginx_generates_only_explicit_trust_directives(self):
        bash = os.environ.get("BASH_EXECUTABLE") or shutil.which("bash")
        if not bash:
            self.skipTest("bash is required to execute the container entrypoint regression")
        script = ROOT / "frontend/docker-entrypoint-trust.sh"
        for raw, expected_code in (("10.42.0.0/16,fd00:42::/64", 0), ("none", 0),
                                   ("0.0.0.0/0", 1), ("0:0:0:0:0:0:0:0/0", 1),
                                   ("::ffff:0.0.0.0/96", 1), ("0:0:0:0:0:FFFF:0:0/96", 1),
                                   ("10.42.0.0/16;include /tmp/evil", 1)):
            with self.subTest(raw=raw), tempfile.TemporaryDirectory(prefix="caiyun-proxy-") as temporary:
                output = Path(temporary) / "trusted.conf"
                env = {**os.environ, "NGINX_TRUSTED_PROXIES": raw, "CAIYUN_NGINX_TRUST_FILE": output.as_posix()}
                process = subprocess.run([bash, script.as_posix()], env=env, capture_output=True, text=True, timeout=5)
                self.assertEqual(process.returncode, expected_code, process.stderr)
                if raw.startswith("10.42") and expected_code == 0:
                    self.assertEqual(output.read_text(), "set_real_ip_from 10.42.0.0/16;\nset_real_ip_from fd00:42::/64;\n")
                if raw == "none":
                    self.assertEqual(output.read_text(), "")


if __name__ == "__main__":
    unittest.main()
