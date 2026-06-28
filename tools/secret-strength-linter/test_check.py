"""Tests for the secret strength linter.

These cover the pure parsing and scoring helpers; `sops` decryption is mocked
so the suite needs neither the `sops` binary nor a decryption key.
"""

import json
from types import SimpleNamespace

import pytest

import check


# --- _is_secret_key ---------------------------------------------------------


@pytest.mark.parametrize(
    "key",
    [
        "PASSWORD",
        "db_password",
        "PASSWD",
        "user_pw",
        "API_SECRET",
        "github_token",
        "MY_CREDENTIAL",
        "tls_private_key",
        "aws_access_key",
        "openai_api_key",
        "BASIC_AUTH",
    ],
)
def test_is_secret_key_true(key):
    assert check._is_secret_key(key)


@pytest.mark.parametrize("key", ["host", "port", "name", "url", "enabled", "region"])
def test_is_secret_key_false(key):
    assert not check._is_secret_key(key)


# --- _parse_env_data --------------------------------------------------------


def test_parse_env_double_quoted():
    assert check._parse_env_data('FOO="bar baz"\n', "x.env") == {"FOO": "bar baz"}


def test_parse_env_single_quoted():
    assert check._parse_env_data("FOO='bar baz'\n", "x.env") == {"FOO": "bar baz"}


def test_parse_env_bare():
    assert check._parse_env_data("FOO=bar\n", "x.env") == {"FOO": "bar"}


def test_parse_env_inline_comment_after_value_is_unparseable(capsys):
    # The value char class excludes '#', but the trailing `\s*$` then can't
    # consume the comment, so such a line is treated as unparseable (exits).
    with pytest.raises(SystemExit) as exc:
        check._parse_env_data("FOO=bar # note\n", "x.env")
    assert exc.value.code == 1
    assert "unparseable line" in capsys.readouterr().err


def test_parse_env_ignores_comment_and_blank_lines():
    content = "# a comment\n\nFOO=bar\n"
    assert check._parse_env_data(content, "x.env") == {"FOO": "bar"}


def test_parse_env_multiple_keys():
    content = 'A=1\nB="two"\nC=three\n'
    assert check._parse_env_data(content, "x.env") == {"A": "1", "B": "two", "C": "three"}


def test_parse_env_unparseable_line_exits(capsys):
    with pytest.raises(SystemExit) as exc:
        check._parse_env_data("this is not valid\n", "x.env")
    assert exc.value.code == 1
    assert "unparseable line" in capsys.readouterr().err


# --- _parse_binary_data routing --------------------------------------------


def test_parse_binary_routes_env(monkeypatch):
    monkeypatch.setattr(check, "_parse_hcl_data", lambda *a: pytest.fail("used hcl"))
    assert check._parse_binary_data("FOO=bar\n", "secrets.sops.env") == {"FOO": "bar"}


def test_parse_binary_routes_hcl_for_tfvars(monkeypatch):
    called = {}

    def fake_hcl(content, source):
        called["yes"] = (content, source)
        return {"parsed": True}

    monkeypatch.setattr(check, "_parse_hcl_data", fake_hcl)
    out = check._parse_binary_data('foo = "bar"\n', "terraform.secrets.tfvars")
    assert out == {"parsed": True}
    assert called["yes"] == ('foo = "bar"\n', "terraform.secrets.tfvars")


def test_parse_hcl_invalid_exits(capsys):
    with pytest.raises(SystemExit) as exc:
        check._parse_hcl_data("this = = invalid", "x.tfvars")
    assert exc.value.code == 1
    assert "HCL parse failed" in capsys.readouterr().err


# --- _walk ------------------------------------------------------------------

STRONG = "correct-horse-battery-staple-9281"
WEAK = "password"


def test_walk_flags_weak_secret():
    failures = []
    check._walk({"db_password": WEAK}, [], failures, "f")
    assert len(failures) == 1
    assert failures[0]["file"] == "f"
    assert failures[0]["key"] == "db_password"
    assert failures[0]["score"] < check._MIN_SCORE


def test_walk_passes_strong_secret():
    failures = []
    check._walk({"db_password": STRONG}, [], failures, "f")
    assert failures == []


def test_walk_ignores_non_secret_keys():
    failures = []
    check._walk({"hostname": WEAK}, [], failures, "f")
    assert failures == []


def test_walk_skips_sops_metadata():
    failures = []
    check._walk({"sops": {"password": WEAK}}, [], failures, "f")
    assert failures == []


def test_walk_ignores_empty_string():
    failures = []
    check._walk({"password": ""}, [], failures, "f")
    assert failures == []


def test_walk_nested_dict_builds_key_path():
    failures = []
    check._walk({"db": {"settings": {"password": WEAK}}}, [], failures, "f")
    assert failures[0]["key"] == "db.settings.password"


def test_walk_descends_into_lists():
    failures = []
    check._walk({"creds": [{"password": WEAK}]}, [], failures, "f")
    assert len(failures) == 1
    assert failures[0]["key"] == "creds.password"


# --- _check_file (sops mocked) ---------------------------------------------


def _mock_sops(monkeypatch, stdout, returncode=0, stderr=""):
    def fake_run(cmd, capture_output, text):
        return SimpleNamespace(returncode=returncode, stdout=stdout, stderr=stderr)

    monkeypatch.setattr(check.subprocess, "run", fake_run)


def test_check_file_json_structure(monkeypatch):
    _mock_sops(monkeypatch, json.dumps({"api_token": WEAK, "host": "example.com"}))
    failures = check._check_file("x.secrets.sops.json")
    assert len(failures) == 1
    assert failures[0]["key"] == "api_token"


def test_check_file_binary_env(monkeypatch):
    payload = json.dumps({"data": f"API_TOKEN={WEAK}\n", "sops": {"version": "3"}})
    _mock_sops(monkeypatch, payload)
    failures = check._check_file("x.secrets.sops.env")
    assert len(failures) == 1
    assert failures[0]["key"] == "API_TOKEN"


def test_check_file_strong_passes(monkeypatch):
    _mock_sops(monkeypatch, json.dumps({"api_token": STRONG}))
    assert check._check_file("x.secrets.sops.json") == []


def test_check_file_sops_failure_exits(monkeypatch, capsys):
    _mock_sops(monkeypatch, "", returncode=1, stderr="no key")
    with pytest.raises(SystemExit) as exc:
        check._check_file("x.secrets.sops.json")
    assert exc.value.code == 1
    assert "sops decrypt failed" in capsys.readouterr().err


# --- main -------------------------------------------------------------------


def test_main_no_files(monkeypatch, capsys):
    monkeypatch.setattr(check.sys, "argv", ["check.py"])
    with pytest.raises(SystemExit) as exc:
        check.main()
    assert exc.value.code == 0
    assert "No SOPS files" in capsys.readouterr().out


def test_main_reports_failures(monkeypatch, capsys):
    monkeypatch.setattr(check.sys, "argv", ["check.py", "x.secrets.sops.json"])
    _mock_sops(monkeypatch, json.dumps({"api_token": WEAK}))
    with pytest.raises(SystemExit) as exc:
        check.main()
    assert exc.value.code == 1
    out = capsys.readouterr().out
    assert "weak secret(s) found" in out
    assert "api_token" in out


def test_main_all_pass(monkeypatch, capsys):
    monkeypatch.setattr(check.sys, "argv", ["check.py", "x.secrets.sops.json"])
    _mock_sops(monkeypatch, json.dumps({"api_token": STRONG}))
    check.main()
    assert "passed strength check" in capsys.readouterr().out
