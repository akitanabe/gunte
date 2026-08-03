<!-- Generated from shared/. Do not edit directly. -->

# ユーザー確認

生成した Branch Plan Set をユーザーへ提示し、確認を得る手順を定める。この Skill は計画の確定までを
担い、委譲は開始しない。確認はあくまで Branch Plan Set の承認であり、委譲開始権限とは独立している。

## 目次

- 提示の順序
- 要約表
- 確認操作
- findings 由来 AC の確定
- blocked の提示
- assumptions と confirmation_mode

## 提示の順序

提示の分岐は Set 全体で決める。

- Branch Plan が1件でも `blocked` であれば、`blocked` の提示を行う。この状態では承認操作を
  求めない。
- 全 Branch Plan が同じ `status` のときは、その `status` の提示を行う。
  - `awaiting_review`: Set の `order` と Branch Plan ごとの要約表 → 確認操作 →
    Branch Plan Set の YAML 全文の順で提示する。要約表は YAML 全文の前に必ず置き、全文を
    読まなくても分割の妥当性を判断できるようにする。
  - `approved`(`method: auto`): 自動承認した記録として Set の `order` と要約表、
    Branch Plan Set を提示し、承認が自動化された範囲(委譲開始は含まない)を明示する。
- `blocked` が0件で `status` が揃わない場合(`awaiting_review` と `approved`(`method: auto`) の
  混在)は、承認操作を求める側の `awaiting_review` の提示に寄せる。`approved`(`method: auto`) の
  Branch Plan は、`shared_foundation.required: true` の注記と同じ置き方で、該当する
  Branch Plan の表の前に `confirmation_mode: auto` により自動承認済みである旨を付記する。

## 要約表

Set の `order` を要約表の前に示し、Branch Plan の実行順序を明示する。Set の `decision.split` が
`false` の場合は、`order` に続けてその理由(`decision.reason`)を提示する。

続けて、YAML 全文の前に、Branch Plan ごとに次の列を持つ要約表を表示する。

| 実行順 | 枝 | 主責務 | テスト | 依存 |
| --- | --- | --- | --- | --- |

- 実行順は、その Branch Plan の `execution.order` の順序に一致させる。
- 主責務は枝の `purpose`(外部から観測可能な振る舞い)を短く示す。
- テストは枝の `tests` の種別を示す。
- 依存は `depends_on` を示す。

`shared_foundation.required: true` の場合は、親が委譲前に実装する共有土台として、該当する
Branch Plan の表の前に明示する。

## 確認操作

ユーザーへ次の3種の操作を、どちらの層に掛かるかを明示して提示する。

- この分割で実行 — Set 全体の承認を意味する。`status: awaiting_review` の Branch Plan について
  `approval.method: user` と `status: approved` を記録し、すでに `approved`(`method: auto`) の
  Branch Plan は変更しない。Set 層に承認状態は持たない。無条件に上書きすると
  `confirmation_mode: auto` の Branch Plan が `approval.method: user` になり、有効な組み合わせ表に
  ない状態を作るため、この上書きを行わない。提示した Branch Plan Set をそのまま確定する。
  遷移条件と実行主体は [Branch Plan Set 正規スキーマ](branch-plan-schema.md) の
  「状態遷移と権限」に従う。
- 分割を修正 — Branch Plan への分割(Set の `branch_plans` の分け方や `order`)か、実装枝への
  分割(Branch Plan 内の `branches` の分け方)か、どちらの層の修正かをユーザーが示す。AC 割り当て
  の修正は枝の `covers_acceptance_criteria` へ反映する。指定された層(Set の `branch_plans` /
  Branch Plan の `branches`)へ反映して validation を再実行してから再提示する。
- 分割せず1枝で実行 — 実装枝の統合を意味する。対象が単一の Branch Plan 内の枝なら、記録先は
  現行どおりその Branch Plan の `override.merge_branches` とする。対象が `branch_plans` が
  2件以上ある Set 全体(Branch Plan を1件へまとめる指示)なら、Set を1件へ畳む指示として扱い、
  Set の `decision.split: false` と、統合後の Branch Plan の `override.merge_branches` の
  両方を記録し、Branch Plan Set を再生成する。

`override` を Set 層へ増やさない。`override` は実装枝の統合という Branch Plan 内の操作を
記録する field であり、層をまたいで意味を広げると記録先が入力によって変わるためである。

承認は Branch Plan Set の確定だけを意味する。委譲開始は、ユーザーの明示的な委譲要求だけを根拠に
親エージェントが `delegation` を設定した後に、`impl-lead` 側で行う。

## findings 由来 AC の確定

Test Inventory 報告の findings を元プランにした場合、AC の文言が確定するまで承認を求めない。

- 対象 `G-*` ごとに、`summary` / `evidence` / `suggestion` の原文と、そこから導出した AC 案を
  対で提示する。導出元を読み直さずに確定を判断できる提示にする。
- 提示する AC 案の文言には、[起草手順](../../plan-craft/references/plan-drafting.md)
  の「AC の書き方」が定める判定可能性の指針を適用する。適用範囲は AC 案の文言整形までに限り、
  `suggestion` にない対象・範囲を新たに足す判断には使わない。対象・範囲を足す必要が生じた
  場合は、下記のとおり `unresolved_decisions` へ回す。
- AC の `text` にはユーザーが確定した文言だけを入れる。提示した AC 案は確定前の提示物であり、
  そのまま採用する場合も確定操作を経る。
- 未確定の間は `unresolved_decisions` に `kind: ac-derivation` の `affects` を置き、
  `status: blocked` のまま承認操作を求めない。
- 文言が確定したら AC の `text` を確定文言に置き換え、対応する `unresolved_decisions` を取り除く。
- `suggestion` にない対象・範囲・実装方針を足す必要が生じた場合は、導出せず `unresolved_decisions` の
  `question` としてユーザーへ確定を求める。

## blocked の提示

Branch Plan Set 内に `status: blocked` の Branch Plan が1件でもあれば、承認操作を求めず、
原因の解消を依頼する。

- Set の `validation.blocking` が非空の場合は、これを先に提示する。Set の違反が全 Branch Plan を
  `blocked` にすることは [Branch Plan Set 正規スキーマ](branch-plan-schema.md) の
  「状態遷移と権限」による。
- 各 blocked な Branch Plan について、`unresolved_decisions` は `question` と `affects` を
  対応付けて提示し、確定が必要な判断をユーザーへ示す。仮定で進めず、確定を待つ。
- 各 blocked な Branch Plan の `validation.blocking` は `code`、`path`、`message` を提示し、
  修正に必要な情報を示す。

原因が解消したら全 validation を再実行し、各 Branch Plan の `confirmation_mode` から
`awaiting_review` または `approved`(`method: auto`)へ遷移させて改めて提示する。

## assumptions と confirmation_mode

- `assumptions` に置いた minor な仮定は、承認を求める前に一覧で明示する。枝構造・実行順序・
  AC 割り当てに影響しない仮定に限り、影響する不足は `unresolved_decisions` として提示する。
- `confirmation_mode: auto` は Branch Plan の承認だけを自動化する。ユーザーが明示した場合のみ
  使い、委譲開始権限は含まない。委譲開始には別途ユーザーの明示的な委譲要求が必要である。
- 既定の確認モードは `review` とする。
