# Tugite oracle fixture

この fixture は Tugite commit `4f014ed2ac6f578f54f0a6f774598fecae3bc36a`
の `scripts/build_plugin_assets.py` が返す全 77 artifact を固定する。oracle repository
の tracked file 全体ではなく、同 commit の `build_outputs()` の返却集合が境界である。
そのため、commit 内には存在するが generator の返却集合にない
`plugins/codex/skills/install-custom-agents/SKILL.md` は意図的に含めない。

## 配置

- `input/shared/`: oracle の `shared/`。platform marker だけを SPEC directive へ意味保存変換した source。
- `input/declarations/*/plugin.json`: oracle generator が入力として読む manifest を、出力先と分離した宣言 Data。
- `input/gunte.toml`: finalized Doc-Rev 10 を適用済みの semantic build input。Issue #1 で採用された語彙を明記して使う。
- `input/contracts.toml`: oracle が artifact 内容へ課す独立 predicate はないため空 registry。
- `golden/`: oracle generator の返却 bytes。
- `ORACLE_OUTPUTS`: `build_outputs()` から取得した相対 path 集合。
- `DIGESTS`: `golden/` 相対 path の byte SHA-256。

## 再生成

既存の Tugite worktree では generator を実行しない。必ず commit を archive 展開する。

```sh
ORACLE_REPO=/home/akitanabe/source/repos/tugite
ORACLE_COMMIT=4f014ed2ac6f578f54f0a6f774598fecae3bc36a
ORACLE_TMP=$(mktemp -d /tmp/gunte-oracle.XXXXXX)
git -C "$ORACLE_REPO" archive "$ORACLE_COMMIT" | tar -x -C "$ORACLE_TMP"
(cd "$ORACLE_TMP" && python3 -B scripts/build_plugin_assets.py)
```

`build_outputs()` の集合を再取得し、既存一覧との差分を先に確認する。

```sh
(cd "$ORACLE_TMP" && python3 -B -c 'import importlib.util,pathlib,sys;p=pathlib.Path("scripts/build_plugin_assets.py").resolve();s=importlib.util.spec_from_file_location("oracle",p);m=importlib.util.module_from_spec(s);sys.modules[s.name]=m;s.loader.exec_module(m);o,e=m.build_outputs(pathlib.Path.cwd());assert not e,e;print("\n".join(sorted(x.relative_to(pathlib.Path.cwd()).as_posix() for x in o)))') > /tmp/gunte-oracle-outputs
diff -u testdata/oracle/ORACLE_OUTPUTS /tmp/gunte-oracle-outputs
while IFS= read -r path; do install -D "$ORACLE_TMP/$path" "testdata/oracle/golden/$path"; done < testdata/oracle/ORACLE_OUTPUTS
(cd testdata/oracle/golden && find . -type f -print0 | sort -z | xargs -0 sha256sum | sed 's#  ./#  #' > ../DIGESTS)
```

`input/shared/` は archive の `shared/` から再コピーし、次の対応表だけを置換する。
`input/declarations/` の2 manifest は archive 内で generator が実際に読み込む manifest
から再コピーする。`shared/terms.toml` の値は `gunte.toml` の `[terms.*]` と照合する。

## marker 変換

| oracle marker | SPEC directive |
|---|---|
| `<!-- claude-only:start -->` | `<!-- @only claude -->` |
| `<!-- claude-only:end -->` | `<!-- @/only -->` |
| `<!-- codex-only:start -->` | `<!-- @only codex -->` |
| `<!-- codex-only:end -->` | `<!-- @/only -->` |

意図的な変換はこの4 marker と、oracle の `shared/terms.toml` を
`gunte.toml` の terms Data に転記したことだけである。manifest は oracle 自身が source
として読む bytes を declarations へ移したもので、golden から逆算していない。

次は変換しない。agent の nested TOML frontmatter、skill の platform 別 YAML frontmatter
と block scalar、本文の改行・placeholder、manifest の object member 順・nested object・array
は oracle input のまま固定する。これにより現行 SPEC の表現不能箇所を input の改変で隠さない。

## Doc-Rev 10 feasibility 判定（fixture固定前 draft の判定履歴）

この節の gap / artifact 別表は、fixture 固定前の Doc-Rev 10 draft に対する feasibility 判定履歴である。
Issue #1 で A1/A2/S1/C1/J1 の5 gapが正本へ採用され、digest 固定により draft は解除済みであり、現在の
fixture input は finalized Doc-Rev 10 を表す。

### gap

| ID | 影響 class | 根拠 | 最小の一般化可能な改訂案 |
|---|---|---|---|
| A1 | Claude/Codex agent 全22件 | source metadata は `[claude]` / `[codex]` nested TOML table。§2.6 の `frontmatter:<key>` は単一 segment だけで nested 値を参照できない。 | §2.6 と §5.1 の `from` を `frontmatter:<segment>(.<segment>)*` に拡張し、各 segment は現行 key 文法、table を順に辿り最終値を logical type として検査する。 |
| A2 | Claude agent 全11件 | oracle の `model` / `effort` は YAML plain scalar。§6.2 の `string` は必ず quoted なので byte 不一致。 | §5.1 の type と §6.2 matrix に YAML 専用 `plain_token` を追加する。値文法を `[A-Za-z][A-Za-z0-9_-]*`、YAML予約語禁止とし、無引用で出力する。 |
| S1 | Claude/Codex skill 全10件 | platform projection 後に YAML frontmatter が先頭になる。parse-before-projection の §2.4/§4.3 では認識できず、§6.3 は block scalar と元の行折返しを再現できない。header もその YAML closing delimiter 直後でなければならない。 | §5.3 の既存 `markdown+yaml-frontmatter-v1` に、metadata が空の場合だけ「projection 後 body 先頭の完全な YAML frontmatter blockをbytes保存し、headerをclosing delimiter直後へ挿入する」preserve branchを追加する。metadata生成 branchとの同時使用は禁止。5 profileは増やさない。 |
| C1 | Codex agent 全11件 | oracle は TOML header の直後に metadata 1行目を置く。§5.3/§6.3 は header 後に空行を必須化する。 | §6.3 `toml-v1` の header を `# <header> LF` のみに改訂する。oracle がこの profile の基準なので設定分岐は追加しない。 |
| J1 | plugin.json 2件 | oracle は source manifest を読み、versionだけを置換する。nested `author` / `interface` と複数行 array があり、§6.3 のflat object・1行arrayでは表現不能。 | §5.3 の `json-v1` bodyを任意のtop-level object sourceとして許可する。metadataは同名top-level memberを元位置で上書きし、新規fieldは宣言順で末尾追加。§6.3をrecursive object/array、2-space indent、objectはsource宣言順、arrayは1要素1行、UTF-8/LF/末尾LFへ拡張する。空bodyは従来どおり `{}`。 |

legacy marker は fixture input 側で正本 directive へ変換できるため SPEC gap ではない。
reference 42件は `markdown-v1`、VERSION 1件は `plain-text-v1` で Doc-Rev 10 のまま
byte一致可能である。placeholder は fixture 内で fence 内に存在せず、oracle と SPEC の
置換範囲差はこの artifact 集合では顕在化しない。

### artifact 別判定

| artifact | fixture固定前 draft | gap |
|---|---|---|
| `plugins/claude/.claude-plugin/plugin.json` | 不能 | J1 |
| `plugins/claude/agents/expert-implementer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/expert-selection-reviewer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/implementer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/over-engineering-reviewer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/plan-adversarial-reviewer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/responsibility-boundary-reviewer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/review-patch-refactorer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/security-side-effect-reviewer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/senior-implementer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/test-quality-reviewer.md` | 不能 | A1, A2 |
| `plugins/claude/agents/writing-principles-reviewer.md` | 不能 | A1, A2 |
| `plugins/claude/skills/branch-design/SKILL.md` | 不能 | S1 |
| `plugins/claude/skills/branch-design/references/branch-plan-schema.md` | 可能 | — |
| `plugins/claude/skills/branch-design/references/branch-splitting.md` | 可能 | — |
| `plugins/claude/skills/branch-design/references/plan-review.md` | 可能 | — |
| `plugins/claude/skills/feature-lead/SKILL.md` | 不能 | S1 |
| `plugins/claude/skills/impl-lead/SKILL.md` | 不能 | S1 |
| `plugins/claude/skills/impl-lead/references/branch-plan-intake.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/branch-review.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/expert-selection.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/finding-routing.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/implementation-branches.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/qa-and-integration.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/qa-report.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/reviewer-dispatch.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/reviewer-findings.md` | 可能 | — |
| `plugins/claude/skills/impl-lead/references/run-closeout.md` | 可能 | — |
| `plugins/claude/skills/plan-craft/SKILL.md` | 不能 | S1 |
| `plugins/claude/skills/plan-craft/references/adversarial-review.md` | 可能 | — |
| `plugins/claude/skills/plan-craft/references/overengineering-plan-review.md` | 可能 | — |
| `plugins/claude/skills/plan-craft/references/plan-artifacts.md` | 可能 | — |
| `plugins/claude/skills/plan-craft/references/plan-drafting.md` | 可能 | — |
| `plugins/claude/skills/test-audit/SKILL.md` | 不能 | S1 |
| `plugins/claude/skills/test-audit/references/gap-catalog.md` | 可能 | — |
| `plugins/claude/skills/test-audit/references/inventory-report.md` | 可能 | — |
| `plugins/claude/skills/test-audit/references/suite-scan.md` | 可能 | — |
| `plugins/claude/skills/test-audit/references/test-inventory-schema.md` | 可能 | — |
| `plugins/codex/.codex-plugin/plugin.json` | 不能 | J1 |
| `plugins/codex/install/VERSION` | 可能 | — |
| `plugins/codex/install/agents/expert-implementer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/expert-selection-reviewer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/implementer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/over-engineering-reviewer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/plan-adversarial-reviewer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/responsibility-boundary-reviewer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/review-patch-refactorer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/security-side-effect-reviewer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/senior-implementer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/test-quality-reviewer.toml` | 不能 | A1, C1 |
| `plugins/codex/install/agents/writing-principles-reviewer.toml` | 不能 | A1, C1 |
| `plugins/codex/skills/branch-design/SKILL.md` | 不能 | S1 |
| `plugins/codex/skills/branch-design/references/branch-plan-schema.md` | 可能 | — |
| `plugins/codex/skills/branch-design/references/branch-splitting.md` | 可能 | — |
| `plugins/codex/skills/branch-design/references/plan-review.md` | 可能 | — |
| `plugins/codex/skills/feature-lead/SKILL.md` | 不能 | S1 |
| `plugins/codex/skills/impl-lead/SKILL.md` | 不能 | S1 |
| `plugins/codex/skills/impl-lead/references/branch-plan-intake.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/branch-review.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/expert-selection.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/finding-routing.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/implementation-branches.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/qa-and-integration.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/qa-report.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/reviewer-dispatch.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/reviewer-findings.md` | 可能 | — |
| `plugins/codex/skills/impl-lead/references/run-closeout.md` | 可能 | — |
| `plugins/codex/skills/plan-craft/SKILL.md` | 不能 | S1 |
| `plugins/codex/skills/plan-craft/references/adversarial-review.md` | 可能 | — |
| `plugins/codex/skills/plan-craft/references/overengineering-plan-review.md` | 可能 | — |
| `plugins/codex/skills/plan-craft/references/plan-artifacts.md` | 可能 | — |
| `plugins/codex/skills/plan-craft/references/plan-drafting.md` | 可能 | — |
| `plugins/codex/skills/test-audit/SKILL.md` | 不能 | S1 |
| `plugins/codex/skills/test-audit/references/gap-catalog.md` | 可能 | — |
| `plugins/codex/skills/test-audit/references/inventory-report.md` | 可能 | — |
| `plugins/codex/skills/test-audit/references/suite-scan.md` | 可能 | — |
| `plugins/codex/skills/test-audit/references/test-inventory-schema.md` | 可能 | — |

## 検証

```sh
go test . -run Oracle
go test ./...
git diff --check
```

テストは DIGESTS と実 bytes、source から導いた generator output 集合、legacy marker 不在、
両 config の TOML parse、oracle commit・再生成情報・全 artifact の README 掲載を検査する。
