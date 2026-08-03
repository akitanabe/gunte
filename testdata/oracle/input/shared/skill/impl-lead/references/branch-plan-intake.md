# Branch Plan の受け入れ

## 目次

- 受け入れ口の規定
- Executor 側の再検証
- 枝 mode の決定表
- Branch Plan 境界の授権

`branch-design` が出力した確定済み Branch Plan Set を
`impl-lead` の入力として受け入れるための規約を定める。親は
Branch Plan の自己申告を信用せず、再検証してから枝と配分方針の入力にする。
Branch Plan Set と Branch Plan の正規スキーマ(スキーマ本体・violation code とその帰属・
Branch Plan の状態遷移)の正本は
[Branch Plan Set 正規スキーマ](../../branch-design/references/branch-plan-schema.md)
であり、本 reference は受け入れ口の規定、Executor 側の再検証、枝 mode の決定表、
Branch Plan 境界の授権の正本を担う。

## 受け入れ口の規定

確定済み Branch Plan Set が渡された場合、親は次の手順で受け入れる。

- 受け入れ対象は Branch Plan Set であり、`order` に従って Branch Plan を順に実行する。
  親や `feature-lead` が Set をほどいて Branch Plan を1つずつ渡す形にはしない。委譲と実行は
  `impl-lead` の責務であり、実行の区切りも同じ skill が持つ。
- 自己申告を信用せず、本 reference「Executor 側の再検証」の5項目を委譲開始前に
  確認する。blocking violation code 表は planning Skill と同じ規則を入力 Data から
  再計算する。code 表の正本は
  [Branch Plan Set 正規スキーマ](../../branch-design/references/branch-plan-schema.md)
  の「blocking violation code」とする。
- 再検証を満たさない場合は実装を開始せず、Branch Plan の修正(または委譲要求の
  有無の確認)を要求する。
- Branch Plan の各枝は、既存の委譲 prompt の Data へそのまま流し込む。目的は
  `purpose`、Acceptance Criteria は `covers_acceptance_criteria` の原文と
  `branch_criteria`、変更禁止範囲は `forbidden_paths`、必須テストは `tests` に対応させる。
  責務制約は `out_of_scope` の各項目を意味を変えず
  委譲 prompt の「この枝でやらないこと」へ渡す。委譲 prompt の構成は
  [実装枝の準備と委譲](implementation-branches.md) に従う。
- 枝の `tests` に列挙された種別が、委譲 prompt の必須テストと検証 command で
  すべて充足されることを委譲前に確認する。テスト種別の意味は正規スキーマの
  「tests の意味」に従う。
- `purpose`、`branch_criteria`、`tests` から、各追加 test が新機能または未実装仕様を検証するのか、
  既存挙動を固定する regression test なのかを委譲前に確認する。後者だけが Red 証跡の Green 例外を
  利用できる。分類できない場合は Green 例外を適用せず判断点として返す。regression と確認できた場合は、
  [実装枝の準備と委譲](implementation-branches.md) の4項目の根拠を返却条件へ追加する。
- Branch Plan Set が渡されていない場合は、現行どおり親が inline に枝を分ける。分割
  シグナルに該当する場合は `branch-design` の使用を推奨する
  (強制しない)。

## Executor 側の再検証

Set 全体の検査を先に行う。対象は
[Branch Plan Set 正規スキーマ](../../branch-design/references/branch-plan-schema.md)
の blocking violation code 表で帰属が `Set` の code と、帰属が `両方` の code の Set 側 field とし、
Set 全体の Data から再計算する。どの code がどちらの帰属かは同表を正本とし、本 reference へ複製しない。
Set の `validation.blocking` が非空なら、Branch Plan 側の状態に関わらず実行を開始しない。
Set は `status` を持たないため、Set 帰属の違反を実行可否へ伝える経路がこの先行検査しかない。

`impl-lead` は Branch Plan の自己申告を信用せず、委譲開始前に次を再検証する。
次の5項目は、実行対象の Branch Plan ごとに繰り返す。Branch Plan は承認と委譲開始権限を
それぞれ独立して持つため、先行 Branch Plan の再検証結果を後続 Branch Plan へ流用できない。

1. `status: approved` であり、`approval.method` が設定済みである。
2. `delegation.authorized: true` かつ `authorized_by: user` である。
3. `unresolved_decisions` が空である。
4. blocking violation code 表のうち、その Branch Plan 帰属の検査規則を入力 Data から再計算し、
   違反が0件である。帰属が `Set` の code は先行検査で扱い、ここでは再計算しない。
   帰属が `両方` の code は、Branch Plan 側 field をここで再計算する。
5. 全枝に `failure_impact` と `implementation_complexity` が存在する。両 field の `level` が
   `low` / `medium` / `high` のいずれかである。両 field の `reasons` が欠落しておらず、非空の
   文字列配列である。欠落、配列以外、空配列、空文字、非文字列要素は
   `branch-assessment-missing` または `branch-assessment-invalid` として委譲を開始せず、
   Branch Plan の修正を要求する。旧 `risk` が単独で存在する場合、または旧 `risk` が新しい field と
   混在する場合は `legacy-risk-present` とする。旧 `risk` から `failure_impact` または
   `implementation_complexity` を推測しない。Branch Plan の修正を要求し、委譲を開始しない。

いずれかを満たさない場合は実装を開始せず、Branch Plan の修正(または委譲要求の有無の確認)を
要求する。ただし項目2 だけが不成立の場合は修正を要求せず、本 reference
「Branch Plan 境界の授権」に従う。授権が未設定であることは Branch Plan の誤りではなく、
境界に到達したことを表すためである。

5項目を満たした後、委譲開始前に枝ごとの mode を導出する。

- `delegation.requested_mode` を入力語彙の写像ではなく Data として受け取り、`null` の場合は
  `{adaptive, standard}` を採用する。
- 「枝 mode の決定表」から枝ごとの mode を再計算する。planning Skill 側の申告や
  `delegation_mode_proposal` の内容を根拠にしない。
- `failure_impact` は枝 mode の直接導出に使わない。
- 導出結果は実行 Data として保持し、Branch Plan へ書き戻さない。

配車は候補抽出と実割当をまとめた親の判断である。再配車は返却後の新しい routing snapshot で worker を再割当する判断である。
現在授権され、5項目の再検証と mode 導出を通過した実行対象 Branch Plan 1件だけを配車母集団とする。
その Branch Plan の全枝を候補抽出する。[実装枝の準備と委譲](implementation-branches.md) の
「候補抽出と実割当」に従い、候補抽出と実割当を分離する。Branch Plan 単位で配車を一括確定する。確定後に委譲を開始する。
未授権の後続 Branch Plan を配車母集団に含めない。ユーザーが全件一括授権した場合も、Branch Plan ごとに
再検証と受入 snapshot を作り、各 Plan の全枝をその Plan の配車として確定する。同一の受入 snapshot 内で候補と
配車を固定する。Implementer の返却後は新しい routing snapshot として再判断する。

## 枝 mode の決定表

配分方針 `policy`、基準 `baseline`、枝の `implementation_complexity.level` から枝ごとの mode を導出する。
この表を正本とし、planning Skill と Executor は同じ表を使う。

| policy | baseline | `implementation_complexity.level: low` | `medium` | `high` |
| --- | --- | --- | --- | --- |
| `fixed` | `lite` | `lite` | `lite` | `lite` |
| `fixed` | `strict` | `strict` | `strict` | `strict` |
| `adaptive` | `standard` | `lite` | `standard` | `strict` |
| `adaptive` | `strict` | `standard` | `strict` | `strict` |

`policy: fixed` では導出を行わず、全枝へ `baseline` をそのまま適用する。`fixed` の2行は、
`implementation_complexity.level` を読まずに `baseline` を適用することを表の上で確認できるように置く。

`{adaptive, strict}` の `low` は `lite` ではなく `standard` とする。「判断に迷う場合は基準側へ
倒す」方針を `strict` baseline では `low` にも適用するのが一貫するためである。この結果
`{adaptive, strict}` は `implementation_complexity.level` の3値に対して2値しか使わず、`{adaptive, standard}` との差は
`medium` だけでなく `low` にも現れる。`{adaptive, strict}` で `lite` が必要な枝は、理由を記録した
手動上書きで降格する。表の側で `low → lite` に戻すと、`strict` を指定したユーザーの意図に反して
low complexity / 実装複雑度の判定誤りが無検証のまま通る。

`shared_foundation` は親が委譲前に実装する明示的な例外であり委譲枝ではないため、枝 mode の
導出対象外とする。親は現行どおり `verification` を実行して基準 commit にする。実行前サマリーの
枝一覧にも配分対象として並べない。

### 手動上書き

導出結果はユーザーまたは親エージェントが上書きできる。上書きは実行 Data であり、Branch Plan の
フィールドではない。

- 引き上げ(`lite → standard`、`standard → strict`)は理由の記録を必須としない。
- 降格は理由の記録を必須とする。理由なしの降格は受け付けない。
- `implementation_complexity.level: high` の枝を `lite` へ直接降格させない。
- 判断材料が不足している場合は `baseline` 側へ倒す。
- 上書きは最終報告に含める。

`implementation_complexity.level` そのものが誤っていると判断した場合は、上書きではなく Branch Plan の
`implementation_complexity` を修正して再検証する。上書きを implementation complexity 修正の代用にしない。
実行 Data 側だけを書き換えると、誤った `implementation_complexity` が Branch Plan に残り、後続枝の
mode 導出がその誤りを根拠に続く。

上書きを受け付ける入力経路は本 reference で規定しない。Branch Plan の受け渡しと同じく親エージェントの
責務とする。

## Branch Plan 境界の授権

Branch Plan 境界の停止は、新しいゲート機構ではなく `delegation.authorized` の再検証で表す。
`delegation.authorized: false` の Branch Plan に到達した時点で実行を止める。
完了済み Branch Plan の最終報告と未実行 Branch Plan の一覧を提示して、その Branch Plan の授権を
要求する。再検証の項目2「`delegation.authorized: true` かつ `authorized_by: user`」が
そのまま境界の判定になる。境界のために独立した状態や field を増やさないのは、承認と委譲開始権限が
すでに Branch Plan ごとに独立して存在し、同じ判定を二重に持つと矛盾したときにどちらを正とするか
決められないためである。

授権の粒度は次のとおりとする。

- 親は1回の委譲要求で全 Branch Plan を授権しない。
- 既定では `order` の先頭の未実行 Branch Plan だけを授権する。
- ユーザーが全 Branch Plan の一括授権を明示した場合だけ全件を授権する。

先行 Branch Plan が完了せずに終了した場合の後続の扱いは
[Run の終了処理](run-closeout.md) を正本とし、授権より優先する。
