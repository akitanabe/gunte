from __future__ import annotations

import hashlib
from pathlib import Path
import re
import tomllib
import unittest


ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / "testdata/oracle"
INPUT = FIXTURE / "input"
GOLDEN = FIXTURE / "golden"
ORACLE_COMMIT = "4f014ed2ac6f578f54f0a6f774598fecae3bc36a"
ORACLE_OUTPUTS_SHA256 = "d4b57079913609cbd7d84e0c47a0e8585c1ba17906a7a083cda8cc8b38af8bea"
DIGESTS_SHA256 = "f392585f469da738b149d6befae579a2eef9fa87c7e72c3c7ab7212a4c4f2612"
SHARED_TREE_SHA256 = "46dc9a58fa00abfe22afa86597ef0f4e6e306bfa2b600282910f4ffcb8361613"
CLAUDE_MANIFEST_SHA256 = "472a1856cdfae5e5a843187047af38b4bbfbf370c92e472822a863b20030babc"
CODEX_MANIFEST_SHA256 = "fd6ed3a787939faccc77b959b35fcbed9215d1ba2bfd73fa7a1869851c01aa85"
LEGACY_MARKER = re.compile(
    rb"<!--[ \t]*(?:claude|codex)-only[ \t]*:[ \t]*(?:start|end)[ \t]*-->"
)
ORACLE_OUTPUT_PATHS = (
    "plugins/claude/.claude-plugin/plugin.json",
    "plugins/claude/agents/expert-implementer.md",
    "plugins/claude/agents/expert-selection-reviewer.md",
    "plugins/claude/agents/implementer.md",
    "plugins/claude/agents/over-engineering-reviewer.md",
    "plugins/claude/agents/plan-adversarial-reviewer.md",
    "plugins/claude/agents/responsibility-boundary-reviewer.md",
    "plugins/claude/agents/review-patch-refactorer.md",
    "plugins/claude/agents/security-side-effect-reviewer.md",
    "plugins/claude/agents/senior-implementer.md",
    "plugins/claude/agents/test-quality-reviewer.md",
    "plugins/claude/agents/writing-principles-reviewer.md",
    "plugins/claude/skills/branch-design/SKILL.md",
    "plugins/claude/skills/branch-design/references/branch-plan-schema.md",
    "plugins/claude/skills/branch-design/references/branch-splitting.md",
    "plugins/claude/skills/branch-design/references/plan-review.md",
    "plugins/claude/skills/feature-lead/SKILL.md",
    "plugins/claude/skills/impl-lead/SKILL.md",
    "plugins/claude/skills/impl-lead/references/branch-plan-intake.md",
    "plugins/claude/skills/impl-lead/references/branch-review.md",
    "plugins/claude/skills/impl-lead/references/expert-selection.md",
    "plugins/claude/skills/impl-lead/references/finding-routing.md",
    "plugins/claude/skills/impl-lead/references/implementation-branches.md",
    "plugins/claude/skills/impl-lead/references/qa-and-integration.md",
    "plugins/claude/skills/impl-lead/references/qa-report.md",
    "plugins/claude/skills/impl-lead/references/reviewer-dispatch.md",
    "plugins/claude/skills/impl-lead/references/reviewer-findings.md",
    "plugins/claude/skills/impl-lead/references/run-closeout.md",
    "plugins/claude/skills/plan-craft/SKILL.md",
    "plugins/claude/skills/plan-craft/references/adversarial-review.md",
    "plugins/claude/skills/plan-craft/references/overengineering-plan-review.md",
    "plugins/claude/skills/plan-craft/references/plan-artifacts.md",
    "plugins/claude/skills/plan-craft/references/plan-drafting.md",
    "plugins/claude/skills/test-audit/SKILL.md",
    "plugins/claude/skills/test-audit/references/gap-catalog.md",
    "plugins/claude/skills/test-audit/references/inventory-report.md",
    "plugins/claude/skills/test-audit/references/suite-scan.md",
    "plugins/claude/skills/test-audit/references/test-inventory-schema.md",
    "plugins/codex/.codex-plugin/plugin.json",
    "plugins/codex/install/VERSION",
    "plugins/codex/install/agents/expert-implementer.toml",
    "plugins/codex/install/agents/expert-selection-reviewer.toml",
    "plugins/codex/install/agents/implementer.toml",
    "plugins/codex/install/agents/over-engineering-reviewer.toml",
    "plugins/codex/install/agents/plan-adversarial-reviewer.toml",
    "plugins/codex/install/agents/responsibility-boundary-reviewer.toml",
    "plugins/codex/install/agents/review-patch-refactorer.toml",
    "plugins/codex/install/agents/security-side-effect-reviewer.toml",
    "plugins/codex/install/agents/senior-implementer.toml",
    "plugins/codex/install/agents/test-quality-reviewer.toml",
    "plugins/codex/install/agents/writing-principles-reviewer.toml",
    "plugins/codex/skills/branch-design/SKILL.md",
    "plugins/codex/skills/branch-design/references/branch-plan-schema.md",
    "plugins/codex/skills/branch-design/references/branch-splitting.md",
    "plugins/codex/skills/branch-design/references/plan-review.md",
    "plugins/codex/skills/feature-lead/SKILL.md",
    "plugins/codex/skills/impl-lead/SKILL.md",
    "plugins/codex/skills/impl-lead/references/branch-plan-intake.md",
    "plugins/codex/skills/impl-lead/references/branch-review.md",
    "plugins/codex/skills/impl-lead/references/expert-selection.md",
    "plugins/codex/skills/impl-lead/references/finding-routing.md",
    "plugins/codex/skills/impl-lead/references/implementation-branches.md",
    "plugins/codex/skills/impl-lead/references/qa-and-integration.md",
    "plugins/codex/skills/impl-lead/references/qa-report.md",
    "plugins/codex/skills/impl-lead/references/reviewer-dispatch.md",
    "plugins/codex/skills/impl-lead/references/reviewer-findings.md",
    "plugins/codex/skills/impl-lead/references/run-closeout.md",
    "plugins/codex/skills/plan-craft/SKILL.md",
    "plugins/codex/skills/plan-craft/references/adversarial-review.md",
    "plugins/codex/skills/plan-craft/references/overengineering-plan-review.md",
    "plugins/codex/skills/plan-craft/references/plan-artifacts.md",
    "plugins/codex/skills/plan-craft/references/plan-drafting.md",
    "plugins/codex/skills/test-audit/SKILL.md",
    "plugins/codex/skills/test-audit/references/gap-catalog.md",
    "plugins/codex/skills/test-audit/references/inventory-report.md",
    "plugins/codex/skills/test-audit/references/suite-scan.md",
    "plugins/codex/skills/test-audit/references/test-inventory-schema.md",
)
EXPECTED_DIRECTIVE_COUNTS = {
    b"<!-- @only claude -->": 9,
    b"<!-- @only codex -->": 12,
    b"<!-- @/only -->": 21,
}


def listed_paths(path: Path) -> list[str]:
    return [line for line in path.read_text(encoding="utf-8").splitlines() if line]


def tree_digest(root: Path) -> str:
    digest = hashlib.sha256()
    for relative_path in sorted(
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_file()
    ):
        digest.update(relative_path.encode("utf-8"))
        digest.update(b"\0")
        digest.update((root / relative_path).read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


class OracleFixtureTest(unittest.TestCase):
    def test_oracle_outputs_is_fixed_anchor(self) -> None:
        outputs = FIXTURE / "ORACLE_OUTPUTS"
        self.assertEqual(hashlib.sha256(outputs.read_bytes()).hexdigest(), ORACLE_OUTPUTS_SHA256)
        self.assertEqual(listed_paths(outputs), list(ORACLE_OUTPUT_PATHS))
        self.assertEqual(len(ORACLE_OUTPUT_PATHS), 77)

    def test_digests_cover_exact_golden_bytes(self) -> None:
        digests_file = FIXTURE / "DIGESTS"
        self.assertEqual(hashlib.sha256(digests_file.read_bytes()).hexdigest(), DIGESTS_SHA256)
        digest_lines = listed_paths(digests_file)
        recorded = {}
        for line in digest_lines:
            digest, relative_path = line.split("  ", 1)
            self.assertRegex(digest, r"^[0-9a-f]{64}$")
            self.assertNotIn(relative_path, recorded)
            recorded[relative_path] = digest

        actual_paths = sorted(
            path.relative_to(GOLDEN).as_posix()
            for path in GOLDEN.rglob("*")
            if path.is_file()
        )
        self.assertEqual(sorted(recorded), actual_paths)
        self.assertEqual(list(recorded), actual_paths)
        for relative_path, expected_digest in recorded.items():
            actual_digest = hashlib.sha256(
                (GOLDEN / relative_path).read_bytes()
            ).hexdigest()
            self.assertEqual(expected_digest, actual_digest, relative_path)

    def test_golden_set_matches_oracle_generator_output_set(self) -> None:
        expected_paths = list(ORACLE_OUTPUT_PATHS)
        self.assertEqual(listed_paths(FIXTURE / "ORACLE_OUTPUTS"), expected_paths)
        self.assertEqual(expected_paths, sorted(expected_paths))
        self.assertEqual(len(expected_paths), len(set(expected_paths)))
        derived_paths = {
            "plugins/claude/.claude-plugin/plugin.json",
            "plugins/codex/.codex-plugin/plugin.json",
            "plugins/codex/install/VERSION",
        }
        for source_path in (INPUT / "shared/agents").glob("*.md"):
            derived_paths.add(f"plugins/claude/agents/{source_path.name}")
            derived_paths.add(
                f"plugins/codex/install/agents/{source_path.stem}.toml"
            )
        for source_path in (INPUT / "shared/skill").rglob("*.md"):
            suffix = source_path.relative_to(INPUT / "shared/skill").as_posix()
            derived_paths.add(f"plugins/claude/skills/{suffix}")
            derived_paths.add(f"plugins/codex/skills/{suffix}")
        self.assertEqual(expected_paths, sorted(derived_paths))
        actual_paths = sorted(
            path.relative_to(GOLDEN).as_posix()
            for path in GOLDEN.rglob("*")
            if path.is_file()
        )
        self.assertEqual(expected_paths, actual_paths)

    def test_shared_input_has_expected_directive_counts_and_no_legacy_markers(self) -> None:
        input_paths = sorted(path for path in INPUT.rglob("*") if path.is_file())
        self.assertTrue(input_paths)
        for path in input_paths:
            self.assertIsNone(LEGACY_MARKER.search(path.read_bytes()), str(path))
        self.assertEqual(tree_digest(INPUT / "shared"), SHARED_TREE_SHA256)
        for marker, expected_count in EXPECTED_DIRECTIVE_COUNTS.items():
            actual_count = sum(
                path.read_bytes().count(marker)
                for path in input_paths
                if path.is_relative_to(INPUT / "shared")
            )
            self.assertEqual(actual_count, expected_count, marker)

    def test_declaration_manifests_have_fixed_raw_bytes(self) -> None:
        self.assertEqual(
            hashlib.sha256((INPUT / "declarations/claude/plugin.json").read_bytes()).hexdigest(),
            CLAUDE_MANIFEST_SHA256,
        )
        self.assertEqual(
            hashlib.sha256((INPUT / "declarations/codex/plugin.json").read_bytes()).hexdigest(),
            CODEX_MANIFEST_SHA256,
        )

    def test_configuration_matches_fixture_sources_terms_and_profiles(self) -> None:
        documents = {}
        for name in ("gunte.toml", "contracts.toml"):
            with self.subTest(name=name):
                document = tomllib.loads((INPUT / name).read_text(encoding="utf-8"))
                self.assertIsInstance(document, dict)
                documents[name] = document

        config = documents["gunte.toml"]
        expected_sources = {
            path.relative_to(INPUT).as_posix()
            for path in (INPUT / "shared").rglob("*")
            if path.is_file() and path.name != "terms.toml"
        }
        expected_sources.update(
            path.relative_to(INPUT).as_posix()
            for path in (INPUT / "declarations").rglob("*")
            if path.is_file()
        )
        sources = config["sources"]["files"]
        self.assertEqual(len(sources), len(set(sources)))
        self.assertEqual(set(sources), expected_sources)
        shared_terms = tomllib.loads((INPUT / "shared/terms.toml").read_text(encoding="utf-8"))
        self.assertEqual(config["terms"], shared_terms["terms"])

        profiles = {
            rule["profile"]
            for target in config["targets"].values()
            for rule in target["rules"]
        }
        self.assertEqual(
            profiles,
            {
                "markdown+yaml-frontmatter-v1",
                "markdown-v1",
                "json-v1",
                "toml-v1",
                "plain-text-v1",
            },
        )
        rules = [
            rule
            for target in config["targets"].values()
            for rule in target["rules"]
        ]
        frontmatter_refs = {
            metadata["from"]
            for rule in rules
            for metadata in rule.get("metadata", [])
            if metadata.get("from", "").startswith("frontmatter:")
        }
        self.assertTrue(frontmatter_refs)
        self.assertTrue(all("." in reference for reference in frontmatter_refs))
        self.assertIn("frontmatter:claude.description", frontmatter_refs)
        self.assertIn("frontmatter:codex.description", frontmatter_refs)
        self.assertTrue(
            any(
                metadata.get("type") == "plain_token"
                and metadata.get("from") in {
                    "frontmatter:claude.model",
                    "frontmatter:claude.effort",
                }
                for rule in rules
                for metadata in rule.get("metadata", [])
            )
        )
        self.assertTrue(any(rule["profile"] == "markdown+yaml-frontmatter-v1" for rule in rules))

    def test_readme_records_reproduction_and_fixture_provenance(self) -> None:
        readme = (FIXTURE / "README.md").read_text(encoding="utf-8")
        self.assertIn(ORACLE_COMMIT, readme)
        self.assertIn("git -C", readme)
        self.assertIn(" archive ", readme)
        self.assertIn("build_plugin_assets.py", readme)
        self.assertIn("sha256", readme.lower())
        self.assertIn("<!-- claude-only:start -->", readme)
        self.assertIn("<!-- @only claude -->", readme)
        for relative_path in ORACLE_OUTPUT_PATHS:
            self.assertIn(f"`{relative_path}`", readme)

        gap_section = readme.split("### gap", 1)[1].split("### artifact 別判定", 1)[0]
        gap_rows = {}
        for line in gap_section.splitlines():
            match = re.fullmatch(r"\|\s*([A-Z]\d)\s*\|\s*([^|]+)\|\s*([^|]+)\|\s*([^|]+)\|\s*", line)
            if match:
                gap_id, impact_class, rationale, proposal = (
                    part.strip() for part in match.groups()
                )
                self.assertNotIn(gap_id, gap_rows)
                self.assertTrue(impact_class)
                self.assertTrue(rationale)
                self.assertTrue(proposal)
                gap_rows[gap_id] = (impact_class, rationale, proposal)
        self.assertEqual(set(gap_rows), {"A1", "A2", "S1", "C1", "J1"})
        self.assertEqual(len(gap_rows), 5)

        artifact_section = readme.split("### artifact 別判定", 1)[1]
        rows = {}
        for line in artifact_section.splitlines():
            match = re.fullmatch(r"\| `([^`]+)` \| ([^|]+) \| ([^|]+) \|", line)
            if match:
                relative_path, status, gap = (part.strip() for part in match.groups())
                self.assertNotIn(relative_path, rows)
                rows[relative_path] = (status, gap)
        self.assertEqual(set(rows), set(ORACLE_OUTPUT_PATHS))
        self.assertEqual(len(rows), 77)
        self.assertIn("A1", artifact_section)
        self.assertIn("A2", artifact_section)
        self.assertIn("S1", artifact_section)
        self.assertIn("C1", artifact_section)
        self.assertIn("J1", artifact_section)
        for relative_path, (status, gap) in rows.items():
            if relative_path.endswith("plugin.json"):
                expected = ("不能", "J1")
            elif "/claude/agents/" in relative_path:
                expected = ("不能", "A1, A2")
            elif "/codex/install/agents/" in relative_path:
                expected = ("不能", "A1, C1")
            elif "/skills/" in relative_path and relative_path.endswith("/SKILL.md"):
                expected = ("不能", "S1")
            else:
                expected = ("可能", "—")
            self.assertEqual((status, gap), expected, relative_path)


if __name__ == "__main__":
    unittest.main()
