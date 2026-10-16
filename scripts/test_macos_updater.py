#!/usr/bin/env python3
"""Native shell tests of updater policy; launchd operations are substituted, never installed."""
import hashlib
import io
from pathlib import Path
import platform
import shlex
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / 'scripts/macos/aiceberg-agent-apply-update.sh'


class MacUpdaterTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aiceberg-macos-updater-test-')
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.binary = self.directory / 'aiceberg_agent'
        self.binary.write_text('old-binary')
        self.package = self.directory / 'package.tar.gz'
        self.candidate = self.directory / 'candidate'
        self.backup = self.directory / 'backup'
        self.calls = self.directory / 'calls'
        self.archive('new-binary')

    def archive(self, content, name=None):
        arch = 'arm64' if platform.machine() == 'arm64' else 'amd64'
        with tarfile.open(self.package, 'w:gz') as archive:
            info = tarfile.TarInfo(name or f'aiceberg-agent-darwin-{arch}/aiceberg_agent')
            data = content.encode()
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))

    def run_shell(self, body):
        values = dict(BIN_DST=self.binary, UPDATE_FILE=self.package, CANDIDATE=self.candidate,
                      BACKUP=self.backup, CALLS=self.calls, PLIST=self.directory / 'agent.plist',
                      UPDATE_VERSION='1.2.3', FROM_VERSION='1.2.2', EXPECTED_VERSION='1.2.3',
                      UPDATE_SHA=hashlib.sha256(self.package.read_bytes()).hexdigest(),
                      HEALTH_URL='http://127.0.0.1:8081/health')
        setup = '\n'.join(f'{key}={shlex.quote(str(value))}' for key, value in values.items())
        command = f'source {shlex.quote(str(SCRIPT))}\n{setup}\n{body}'
        return subprocess.run(['/bin/bash', '-c', command], capture_output=True, text=True, timeout=10)

    def test_valid_package_and_successful_restart(self):
        result = self.run_shell('''
service_command() { printf '%s\n' "$*" >>"$CALLS"; }
wait_healthy() { [[ "$EXPECTED_VERSION" == 1.2.3 ]]; }
validate_input && stage_binary && apply_binary
''')
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual('new-binary', self.binary.read_text())
        self.assertEqual('old-binary', self.backup.read_text())
        self.assertEqual(['bootout system/com.aiceberg.agent', 'bootstrap system ' + str(self.directory / 'agent.plist')], self.calls.read_text().splitlines())

    def test_restart_failure_restores_old_binary_and_reports_failure(self):
        result = self.run_shell('''
service_command() { printf '%s\n' "$*" >>"$CALLS"; }
wait_healthy() { [[ "$EXPECTED_VERSION" == 1.2.2 && "$(cat "$BIN_DST")" == old-binary ]]; }
validate_input && stage_binary && apply_binary
''')
        self.assertNotEqual(0, result.returncode)
        self.assertEqual('old-binary', self.binary.read_text())
        self.assertIn('Rollback concluído', result.stdout)
        self.assertEqual(4, len(self.calls.read_text().splitlines()))

    def test_stop_failure_does_not_replace_binary(self):
        result = self.run_shell('service_command() { return 1; }; validate_input && stage_binary && apply_binary')
        self.assertNotEqual(0, result.returncode)
        self.assertEqual('old-binary', self.binary.read_text())

    def test_invalid_hash_rejects_before_stop(self):
        result = self.run_shell('UPDATE_SHA=' + '0' * 64 + '; validate_input')
        self.assertNotEqual(0, result.returncode)
        self.assertFalse(self.calls.exists())

    def test_external_health_url_is_rejected(self):
        result = self.run_shell('HEALTH_URL=https://example.test/health; validate_input')
        self.assertNotEqual(0, result.returncode)

    def test_path_traversal_and_foreign_platform_are_not_extracted(self):
        for name in ['../../aiceberg_agent', 'aiceberg-agent-linux-amd64/aiceberg_agent']:
            self.archive('bad', name)
            result = self.run_shell('validate_input && stage_binary')
            self.assertNotEqual(0, result.returncode)
            self.assertFalse(self.candidate.exists())
            self.assertEqual('old-binary', self.binary.read_text())

    def test_symlink_input_is_rejected(self):
        linked = self.directory / 'linked.tar.gz'
        linked.symlink_to(self.package)
        result = self.run_shell(f'UPDATE_FILE={shlex.quote(str(linked))}; validate_input')
        self.assertNotEqual(0, result.returncode)

    def test_local_health_requires_matching_version(self):
        result = self.run_shell('''
service_command() { return 0; }
health_version() { echo 1.2.2; }
sleep() { :; }
wait_healthy
''')
        self.assertNotEqual(0, result.returncode)


if __name__ == '__main__':
    unittest.main()
