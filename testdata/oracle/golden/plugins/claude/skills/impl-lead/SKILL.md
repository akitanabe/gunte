---
name: impl-lead
description: >-
  実装作業をサブエージェントに委譲しつつ、親エージェントが計画、受け入れ条件、worktree 隔離、
  返却 diff の QA、テスト網羅性レビュー、副作用と責務境界の確認、最終検証、最終報告の責任を
  持つためのワークフロー。ユーザーが実装委譲、マネージャーとしての進行、サブエージェント分担を
  求めたとき、または `lite` / `standard(-adaptive)` / `strict(-adaptive)` / `strict-full` を
  明示したときに使う。`direct` の明示時や、委譲指示なしにタスク規模だけを理由として使わない。
  ユーザーからプランから実装までの一括実行を直接要求された場合、および確定済みの
  プラン文書とレビュー状態を渡して実装までの一括実行を直接要求された場合は、`feature-lead` の
  責務であり発火しない。`feature-lead` の段として起動された場合はこの条件の対象外であり、
  通常どおり動作する。
---
<!-- Generated from shared/. Do not edit directly. -->

# マネージャー＋QA としての委譲

親はタスク分割、Acceptance Criteria、委譲指示、返却 diff の QA、統合、最終検証を担当する。
Implementer は実装を担当するが、最終的な品質責任と受け入れ判断は親から移動しない。

## workflow mode の選択

委譲の決定は次の3層に分ける。層をまたいで並列に選ばない。

1. 経路の選択 — `direct`（この skill の外）か、委譲（この skill）か。
2. 配分方針の選択 — 委譲する場合に、配分方針 `policy` と基準 `baseline` を決める。
3. 枝 mode の導出 — `policy`、`baseline`、枝の `implementation_complexity.level` から枝ごとの mode を導く。

```text
配分方針  policy   : fixed | adaptive
基準      baseline : lite | standard | strict
枝 mode            : lite | standard | strict   ← policy / baseline と枝の implementation_complexity.level から導出する
```

`adaptive` は新しい実装フローではなく、既存の `lite` / `standard` / `strict` を枝へ割り当てる配分方針である。
枝へ割り当てられた後は、その枝を既存の各 mode のフローで実行する。

### 入力語彙の写像

| ユーザー入力 | policy | baseline | 意味と選択条件 |
| --- | --- | --- | --- |
| `direct` | — | — | この skill を発火しない skill 外の経路。委譲要求がなく、仕様が明確で影響範囲が閉じ、親が直接処理する変更。 |
| 指定なし | `adaptive` | `standard` | 通常利用のデフォルト。mode 未指定の明示的な委譲でもこれを選ぶ。 |
| `standard` / `standard-adaptive` | `adaptive` | `standard` | 通常の実装委譲。 |
| `strict` / `strict-adaptive` | `adaptive` | `strict` | 全体として厳格な確認を要求するが、明らかに低 complexity の枝まで一律 `strict` にしない。`standard-adaptive` より保守的に導出する。 |
| `strict-full` | `fixed` | `strict` | 全枝へ `strict` を固定適用する。枝ごとの導出を行わない。 |
| `lite` | `fixed` | `lite` | 全枝を軽量フローで処理する。枝ごとの導出を行わない。ユーザーが明示し、仕様が明確で影響範囲が局所的、容易に戻せる変更にだけ選ぶ。 |

`policy: fixed` は、全枝固定であることを明示的に表現する語彙だけに割り当てる。それ以外の語彙と
mode 未指定はすべて `adaptive` へ写す。今後語彙を追加する場合の既定も `adaptive` とする。

`lite` は名前で全枝固定を表さないため、この表で `{fixed, lite}` と定義することで担保する。`lite` の
adaptive 化が必要になった場合の変更対象は、語彙の名前ではなくこの定義である。`lite` を `adaptive` の
`baseline` にはしない。`baseline` を `lite` にすると low complexity 枝の割り当て先が `lite` しかなく導出が
恒等写像になり、medium 以上を引き上げる用途は `{adaptive, standard}` と同一になるため、独立した
配分方針として意味を持たない。

### 経路の選択

`direct` は親が実装する、この skill の外にある経路である。委譲 mode ではないため、配分方針や枝 mode と
同じ層に並べて選ばない。委譲要求がなく `direct` も指定されていない場合、
タスク規模だけでこの skill を発火しない。`direct` が明示された場合も、この skill を発火しない。
`direct` でも、親は必要なテストと検証を実行し、diff review と最終報告を行う。

ユーザーからプランから実装までの一括実行を直接要求された場合、および確定済みの
プラン文書とレビュー状態を渡して実装までの一括実行を直接要求された場合は、`feature-lead` の
責務であり発火しない。`feature-lead` の段として起動された場合はこの条件の対象外であり、
通常どおり動作する。

### 配分方針の選択

`lite` / `standard(-adaptive)` / `strict(-adaptive)` / `strict-full` の明示は委譲要求を兼ねる。
委譲だけが明示され mode が指定されていない場合は `{adaptive, standard}` を選ぶ。`lite` を自動選択しない。
`direct` と委譲が同時に指定された場合は、実装前にユーザーへ確認する。`direct` から委譲へ変更する場合は、
ユーザーへ確認する。

### 枝 mode の導出

`policy: adaptive` では、`baseline` と枝の `implementation_complexity.level` の決定表で枝ごとの mode を導出する。
決定表の正本は [Branch Plan の受け入れ](references/branch-plan-intake.md) とする。
`policy: fixed` では導出を行わず、全枝へ `baseline` をそのまま適用する。

adaptive の枝 mode は `implementation_complexity.level` だけから導出する。`failure_impact` は adaptive mode の
直接導出に使わない。`failure_impact` は `{fixed, lite}` の安全性に関する `delegation_mode_proposal` に使う。
mode の理由の正本は `implementation_complexity.reasons` とする。

委譲 mode の強度は `lite < standard < strict` とする。導出した枝 mode は Branch Plan へ書き戻さず、
実行 Data として保持して最終報告で報告する。

### 引き上げと引き下げの契約

引き下げ禁止の対象は配分方針 `{policy, baseline}` とする。ユーザーが明示した `baseline` を親都合で
引き下げない。`policy` を親都合で `fixed` から `adaptive` へ変えない。

`feature-lead` の経路で、写像した `requested_mode` が `branch-design` の branch-plan-schema.md の
出力条件表が proposal を要求する組み合わせになる場合に、表が提案する `{policy, baseline}` を
設定することは、この親都合の変更に含めない。ユーザーが mode を明示して一括実行を要求したことが、
この引き上げの授権を兼ねる。引き上げ先は出力条件表に委ね、この原稿で別の値を選ばない。
引き上げ前後の `{policy, baseline}` を記録し、引き上げが生むリスクをユーザーへ報告する。
proposal の安全性判断は `failure_impact.level` を入力にする。

枝への mode 割り当ては決定表による導出結果であり、引き下げに当たらない。導出表を逸脱した割り当てだけを
引き上げ / 引き下げとして扱う。

mode を引き上げた場合は、その具体的な実装複雑度をユーザーへ報告する。導出結果より高い mode で枝を実行する
場合も、枝単位で具体的な実装複雑度をユーザーへ報告する。`lite` の選択条件を満たさなくなった場合は
`standard` 以上へ引き上げる。`standard` では扱えない実装複雑度が判明した場合は `strict` へ引き上げる。
仕様が曖昧な場合は mode を選ぶ前に実装を止め、ユーザーへ確認する。

## 実行前サマリー

実行前サマリーは Branch Plan 単位で提示する。
Branch Plan ごとに配分方針、枝 mode ごとの件数、枝一覧を提示する。
提示のたびにユーザーがその Branch Plan の実行コストを判断できるようにするためであり、
Set 全体の枝一覧を1回だけ提示する形にはしない。

導出後、委譲開始前に次を提示する。

- 解決後の配分方針。`strict` を指定したユーザーが、その場で `strict-adaptive` として解釈されたことを
  確認できるようにする。
- 枝 mode ごとの件数。
- 各枝の `failure_impact.level`、`implementation_complexity.level`、導出した mode、手動上書きの有無。

```text
Mode: standard-adaptive  (policy: adaptive / baseline: standard)

Branch allocation:
  strict   1
  standard 3
  lite     1

1. authorization-check  impact:high / complexity:high    → strict
2. domain-logic         impact:low  / complexity:medium  → standard
3. repository-update    impact:medium / complexity:medium → standard
4. api-response         impact:medium / complexity:low     → lite → standard  (override)
5. label-text           impact:low / complexity:low        → lite
```

枝 mode ごとの件数は、手動上書き後の実効 mode を集計する。実行コストは実効 mode で決まるため、
導出 mode の集計では提示の目的を果たさない。各枝の行では、上書きがある場合に
「導出 mode → 上書き後の mode」の両方を示す。

枝 mode ごとの件数は常に提示する。`strict` 枝が一定数を超えた場合の段階警告は持たない。閾値の根拠が
単一事例であり、枝の重さと repository 規模に依存するため、固定値を契約として持てない。明示的な判断を
促す目的は件数の常時提示で足りる。

### `strict-full` の確認ゲート

`strict-full`（`{fixed, strict}`）は枝数に比例してコストが増えるため、枝数を明示したユーザー確認を
委譲開始条件とする。確認が得られるまで委譲を開始しない。

`strict-full` の確認ゲートは Branch Plan 単位で行う。枝数に比例してコストが増えるという根拠が
Branch Plan 単位で成立するためである。

## 委譲環境の前提

各実装枝は専用 worktree で隔離する。worktree を用意できない場合は委譲を開始しない。
委譲機構または必要な agent が利用できない場合は、利用不能の内容と未着手範囲を報告する。
ユーザーの確認なく親の直接実装へ切り替えない。


## 全体の流れ

1. 目的、入力、出力、Acceptance Criteria、禁止範囲を確定する。
2. 配分方針を選び、共有土台と直列に受け入れる実装枝へ分け、枝ごとの mode を導出する。
   確定済み Branch Plan Set が渡されている場合は
   [Branch Plan の受け入れ](references/branch-plan-intake.md) を読み、再検証してから枝と
   配分方針の入力にする。`order` に従って Branch Plan を順に実行する。以降の手順は、
   いま実行している Branch Plan を対象にする。
3. [実装枝の準備と委譲](references/implementation-branches.md) を読み、基準、worktree、Worker 選択、
   委譲 prompt を準備する。この時点では起動しない。
4. expert 候補の場合だけ、起動前に [Expert 選択](references/expert-selection.md) を読む。
5. 実行前サマリーを提示する。提示と `strict-full` の確認は、いま実行している Branch Plan を
   単位とする。`strict-full` では枝数を明示したユーザー確認を得るまで委譲を開始しない。
   審査と準備が完了した先頭の枝だけを委譲する。
6. Implementer の返答を待ち、返却 commit と実行結果を受け取る。
7. [返却の QA と統合](references/qa-and-integration.md)、
   [枝レビューの進行](references/branch-review.md)、
   [Finding の修正 routing](references/finding-routing.md) を読み、diff、テスト、専門 review、修正経路を判断し、
   その枝のレビューを initial レビュー群・レビューループ・最終レビュー群・完了レビュー群の4相で進める。どの相でどのゲートを
   起動するか、1 round の数え方、打ち切り条件、枝の受け入れ点は「枝レビューの進行」を正本とする。
   reviewer の findings は [Reviewer findings の共通契約](references/reviewer-findings.md) が要求する
   指摘件数のサマリ行と指摘ごとの evidence を備えた形で受け取る。
8. QA 修正を続ける場合は手順7の修正経路を継続する。「枝レビューの4相」の枝の受け入れ点を満たして受け入れる
   場合は1枝だけを統合し、統合後の green を確認して、その commit を次の枝の基準にする。次の枝があれば手順3へ戻る。
   いま実行している Branch Plan の全枝を完了した場合は、`order` に未実行の Branch Plan が残っていても手順9へ進む。
   親が未統合の枝について `Rejected` / `Needs revision` を最終判断とし、top-level workflow を
   終了する場合は、手順9へ進む。
9. 全枝を完了した場合、または手順8で未統合のまま終了する場合は、
   [Run の終了処理](references/run-closeout.md) に従い、適用可能な統合済み diff review と
   最終検証を行い、親の最終判断を確定する。
10. [Run の終了処理](references/run-closeout.md) に従い、最終 gate 後に、
    各 worker worktree の cleanup の実施可否と結果を確定する。
11. 永続 QA レポートの出力条件を満たす場合だけ
    [永続 QA レポート](references/qa-report.md) を読む。
12. [Run の終了処理](references/run-closeout.md) に従い、会話上の最終報告を行う。
    採用した配分方針と枝ごとの mode を含める。ここからは Set 単位で判断する。
    `order` に未実行の Branch Plan が残る場合は、次の Branch Plan の授権を確認する。未授権の Branch Plan に到達した場合は実行を止め、
    [Branch Plan の受け入れ](references/branch-plan-intake.md) に従って完了済みの最終報告と
    未実行 Branch Plan の一覧を提示し、授権を要求する。
    授権された未実行の Branch Plan があれば手順2へ戻り、その Branch Plan を実行する。

共有土台の作成は、実装枝の委譲前に親が行える明示的な例外とする。複数枝が同じ fixture、設定、
テストデータ、生成物を必要とするときだけ先に確定し、検証して基準 commit にする。この例外は、
返却後の機能修正を親が引き取る根拠にはしない。

全ての委譲 mode で、親の最終判断を省略しない。受け入れた枝では統合後の検証を省略しない。
