---
name: plan-craft
description: >-
  ユーザー要求から実装プランを起草し、`plan-adversarial-reviewer` の敵対的レビューループと
  `over-engineering-reviewer` のプラン審査を経たプラン文書とレビュー状態を返す planning skill。
  レビュー付きプラン作成の明示要求時に使う。実装・委譲・枝分割は行わず
  `branch-design` を直接起動しない。次工程の開始権限は含まない。
  ユーザーからプランから実装までの一括実行を直接要求された場合、および確定済みの
  プラン文書とレビュー状態を渡して実装までの一括実行を直接要求された場合は、`feature-lead` の
  責務であり発火しない。`feature-lead` の段として起動された場合はこの条件の対象外であり、
  通常どおり動作する。
---
<!-- Generated from shared/. Do not edit directly. -->

# 実装プランの起草とレビュー

ユーザー要求から実装プランを起草し、敵対的レビューループと過剰実装審査を経たプラン文書とレビュー
状態へ確定する。この Skill はプランの起草と確定までを担い、実装・委譲・枝分割は行わない。確定した
2 artifact は `branch-design` へ渡せるが、受け渡しは親 Codex エージェントの責務で
あり、この Skill は `branch-design` を直接起動しない。

## この Skill の責務

- 出力はプラン文書とレビュー状態の2 artifact だけである（[プラン artifact](references/plan-artifacts.md)）。
  実装、テスト作成、実装枝への分割、worktree 準備、Worker 起動は行わない。
- 承認と次工程の開始権限は独立している。この Skill が扱うのはプランの確定までであり、
  枝分割・委譲の開始権限は含まない。次工程はユーザーの明示的な要求だけを根拠に、親 Codex エージェントが
  後から開始する。
- 要求の不足を勝手に補完しない。AC 充足・scope・実行可否に影響する
  blocking な不足はプラン文書の「依存 / 制約 / 前提 / 未確定」節に確定が必要な問いとして書き、
  影響しない minor な不足は同じ節に仮定として明示する。

## 発火条件

- ユーザーがレビュー付きの実装プラン作成を明示的に要求したとき。

次の場合は発火しない。

- 枝分割計画の要求（`branch-design` の責務）。
- 実装・委譲の要求（`impl-lead` の責務）。
- レビューを求めない相談・調査・回答だけの要求。

ユーザーからプランから実装までの一括実行を直接要求された場合、および確定済みの
プラン文書とレビュー状態を渡して実装までの一括実行を直接要求された場合は、`feature-lead` の
責務であり発火しない。`feature-lead` の段として起動された場合はこの条件の対象外であり、
通常どおり動作する。

## 入力の確認

着手前に次を確認する。不足が blocking なら補完せず、プラン文書の
「依存 / 制約 / 前提 / 未確定」節に確定が必要な問いとして書く。

- 要求原文（言い換えずに保持する）。
- 対象 repository と読み取り可能な現状。
- 既知の制約・依存。
- `rounds_limit` の明示指定。`rounds_limit` の既定は 10 とし、ユーザーが明示した場合のみ変更する。
- 確認モードの既定は `review` とし、`auto` はユーザーが明示した場合のみ使う。

## 全体の流れ

1. 上の入力を確認する。
2. [起草手順](references/plan-drafting.md) に従い、安定 ID 付きで観測可能な振る舞いを表す AC、
   scope、依存、制約を持つプラン文書を起草し、
   [プラン artifact](references/plan-artifacts.md) の保存規約に従って書き出す。
3. [敵対的レビューループ](references/adversarial-review.md) に従い、`plan-adversarial-reviewer` に
   よる round を打ち切り条件が成立するまで繰り返す。採用した指摘を反映するたびにプラン文書を
   上書きする。
4. adversarial の収束後、[過剰実装のプラン審査](references/overengineering-plan-review.md) に従い
   `over-engineering-reviewer` をプラン入力モードで起動する。指摘採用でプランを修正した場合は
   手順3へ戻る。
5. [プラン artifact](references/plan-artifacts.md) に従いレビュー状態を生成する。
6. `validation.blocking` を確定する。表A をレビュー状態 Data から再計算し、表B をプラン文書に
   対する判定で生成する。
7. `open_questions` と `validation.blocking` から `status` を決める。いずれかが非空なら `blocked`。
   空で `confirmation_mode: review` なら `awaiting_review`、`auto` なら `approved`（`method: auto`）。
   ただし `termination: round-limit` で `resolution: unresolved` の指摘が残る場合は、`auto` でも
   自動承認しない。
8. 未解決一覧（あれば）→ 指摘台帳の要約 → レビュー状態の YAML 全文の順で提示する。

## レビューと権限境界

- reviewer は指摘 Data だけを返す。verdict の確定、採用・不採用の判断、プランへの反映は
  親 Codex エージェントが行い、reviewer の自己申告をそのまま採用しない。
- 承認はプランの確定だけを意味する。枝分割は、確定した2 artifact を
  親 Codex エージェントが `branch-design` へ渡した後に、あちら側で行う。この Skill は
  `branch-design` を直接起動しない。
- 会話内経路は同一会話内で完結する用途に限り、後日渡す経路を持たない。正本は
  [プラン artifact](references/plan-artifacts.md) の「file 出力と会話内経路」とする。
