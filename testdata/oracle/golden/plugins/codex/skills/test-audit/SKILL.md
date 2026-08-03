---
name: test-audit
description: >-
  既存テストスイートまたは指定スコープを read-only で走査し、各テストの目的・観測可能な振る舞い・
  分類を Test Inventory Data として棚卸しし、テスト設計技法の観点で不足を安定 ID 付き指摘 Data と
  Markdown 報告で返す。ユーザーがテストの棚卸し・一覧化・gap 分析を明示的に要求したときに使う。
  コードやテストの修正、テスト実行、diff スコープのテストレビュー、受け入れ判断は行わない。
---
<!-- Generated from shared/. Do not edit directly. -->

# テストスイートの棚卸しと gap 分析

既存テストスイートまたは指定スコープを read-only で走査し、各テストの目的・観測可能な振る舞い・
分類を Test Inventory Data として一覧化したうえで、テスト設計技法（同値分割・境界値分析・異常系
網羅）の観点で不足を安定 ID 付き findings として報告する。この Skill は棚卸しと gap 分析だけを担い、
コードやテストの修正、テスト実行、受け入れ判断は行わない。

## この Skill の責務

- 出力は Test Inventory Data と Markdown 報告だけである。テストの追加・修正・削除、テストの実行、
  gate 判定は行わない。
- 走査は read-only とする。テストを実行して振る舞いを確認することはせず、テストコードの読解だけ
  から目的と観測している振る舞いを判断する。
- 特定できない項目は空欄で埋めず、`unknown` や `null` などスキーマが定める値で明示する。読めなかった
  範囲は `unscanned` に記録し、黙って対象から落とさない。
- gap 指摘は observed な `inventory` の事実だけを根拠にする。実装コードを読んで期待仕様を新たに
  補完し、その仕様不足を根拠に findings を作らない。仕様の不足そのものはこの Skill の対象外である。
- 推奨対応(`suggestion`)は Data 上の記述に留め、テストの追加・修正としては実施しない。

## 発火条件

- ユーザーがテストの棚卸し・一覧化・gap 分析を明示的に要求したとき。

次の場合はこの Skill を発火しない。

- diff スコープのテストレビューや受け入れ判断が目的のとき(`test-quality-reviewer` の責務)。
- テストの過剰・重複の削減が目的のとき(`over-engineering-reviewer` の責務)。
- 実装や委譲そのものが目的で、棚卸しの明示要求がないとき。

## 入力の確認

着手前に次を確認する。軽微な曖昧さは仮定を明示して進め、走査結果が大きく変わる曖昧さは仮定で
補完せずユーザーへ確認する。置いた仮定は報告の概況欄に明示する。

- 走査対象スコープ(repository 全体 / 指定ディレクトリ / 指定ファイル)。
- 対象外にするパス(あれば)。
- 報告の粒度や追加観点の希望(あれば)。

## 全体の流れ

1. 上の入力を確認し、`scope.requested` を確定する。
2. [走査手順](references/suite-scan.md) に従い、テストファイルを列挙し読解する。fan-out する場合も
   同じ手順に従う。読めなかった範囲は `unscanned` に記録する。
3. [Test Inventory Data 正規スキーマ](references/test-inventory-schema.md) に従い、各テストへ安定 ID
   を付与し `inventory` を作成する。
4. [gap 観点カタログ](references/gap-catalog.md) に従い、`inventory` を対象へ観点を適用し、安定 ID
   付きの `findings` を作成する。
5. blocking violation code 表を `inventory` と `findings` から再計算し、`validation.blocking` と
   `status` を確定する。
6. [報告形式](references/inventory-report.md) に従い、標準テンプレートで報告する。

## 権限境界

走査は読み取りのみで行い、書き込みや実行は行わない。fan-out するワーカーも read-only とし、
テストを実行しない契約は親 Codex エージェントと共通にする。ワーカーへ渡す範囲もファイル・ディレクトリ
の割り当てに限り、書き込み権限は与えない。走査は既存コードの読解に留まり diff や差し戻しを伴わない
ため、worktree による隔離は不要である。理由は
[走査手順](references/suite-scan.md) の「worktree を作らない理由」を参照する。
