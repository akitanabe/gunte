# Branch Plan Set 正規スキーマ

`branch-design` Skill の出力であり、`impl-lead` への入力となる Branch Plan Set の
正規スキーマ(正本)を定義する。Set は Branch Plan の配列を持つ最上位の Data であり、
確定済みの Set を `impl-lead` へ渡せるが、受け渡しは親エージェントの責務であり、この Skill は
委譲を開始しない。設計の経緯と確定事項は
[issue #46](https://github.com/akitanabe/tugite/issues/46) と
[issue #68](https://github.com/akitanabe/tugite/issues/68) と
[issue #120](https://github.com/akitanabe/tugite/issues/120) と
[issue #122](https://github.com/akitanabe/tugite/issues/122) を参照。

## 目次

- 設計方針
- スキーマ本体
- blocking violation code
- 状態遷移と権限
- tests の意味

## 設計方針

- 実装枝の契約(外部から観測可能な振る舞い単位、単独の Acceptance Criteria・検証・受け入れ判断・
  revert)は `impl-lead` の現行契約を変更せずそのまま使う。実装枝・git branch・
  Branch Plan の用語の書き分けも同じ正本
  [実装枝の準備と委譲](../../impl-lead/references/implementation-branches.md)の
  「用語」節に従う。
- 出力の最上位は Branch Plan Set とし、元プランの `implementation_plan` と
  `acceptance_criteria` は Set が持つ。Set は状態を持たず、`status` / `approval` /
  `delegation` と完了判定はすべて Branch Plan 側に残す。Set に状態を持たせると、受け入れ判断と
  委譲開始権限が Branch Plan 単位の判断と二重管理になり、矛盾したときにどちらを正とするか
  決められないため。
- `branch_plans` が1件の場合も Set を返す。出力形が入力によって変わると、検査の帰属まで
  入力形によって変わるため。
- Set の `order` は Branch Plan id の全順序であり、この並び自体が実行順序の制約である。先行する
  Branch Plan 全体の完了を待ってから次の Branch Plan を開始する。Set 層は `depends_on` のような
  別建ての依存 field を持たないため、並びと突き合わせる依存関係が存在せず、Set 層の `order` の
  検査は `branch_plans[].id` を1回ずつ含むかだけになる。枝どうしの `depends_on` は
  Branch Plan 内で閉じる。
- `decision` は Set と Branch Plan の両方に同名で存在する。層が参照 path(`decision` と
  `branch_plans[].decision`)で一意に決まるため、名前を分けない。
- AC の割り当ては枝側の一方向参照だけにする。AC 側と枝側の両方に割り当てを書くと二重管理になり、
  validation がどちらを正とするか決められないため。この正規化により「未割り当て AC」と
  「primary 不在の AC」は同一の検査に縮退する。
- 枝ごとの委譲 mode は schema に持たせず、`branches[].implementation_complexity` を正として導出する。枝側に
  `recommended_mode` を置くと `implementation_complexity` と二重管理になり、矛盾したときにどちらを正とするか
  決められないため。AC 割り当てを枝側の一方向参照へ正規化したのと同じ理由である。
  導出した枝 mode は Branch Plan へ書き戻さず、実行 Data として保持して最終報告で報告する。
  mode の判定理由は `implementation_complexity.reasons` に書き、mode ごとの理由欄を別に設けない。
- Branch Plan の承認(`approval`)と委譲開始権限(`delegation`)は独立した Data とする。承認は
  計画の確定だけを意味し、委譲開始はユーザーの明示的な委譲要求だけを根拠に親エージェントが
  権限を設定する。
- Set と Branch Plan の `validation.blocking` は、どちらも安定した code を持つ violation の配列とし、
  planning Skill と Executor が同じ検査規則を共有する。承認可否は blocking violation の有無だけで
  決まり、自己評価 boolean は参考情報に限定する。
- `forbidden_paths` は変更を禁止する物理的なファイル範囲を表す。`out_of_scope` はこの枝では担当
  しない責務・作業を表し、パス制約とは独立して扱う。
- Test Inventory 報告の findings から導出した AC は、由来する finding ID(`G-*`)を
  `acceptance_criteria[].derived_from` に記録する。棚卸し報告までの追跡は
  実装枝 → `covers_acceptance_criteria` → AC → `derived_from` の一方向参照でたどる。
  実装枝側に finding ID を持たせない。枝側にも持つと AC 割り当てと二重管理になり、矛盾したときに
  どちらを正とするか決められないため。
- `derived_from` は blocking violation code の検査対象にしない。`G-*` は Branch Plan の外側にある
  Test Inventory Data への参照であり、Branch Plan 内では参照先の存在を解決できない。解決できない
  参照を `unknown-reference` の対象に見せると、承認可否の判定が実際には検査していない事実を根拠に
  持つため。由来の妥当性は、AC の文言を確定するユーザー確認で担保する。

## スキーマ本体

```yaml
# ============================================================
# Branch Plan Set 正規スキーマ (branch-design の出力)
# ============================================================

implementation_plan:
  summary: <実装目的の1行要約>
  source: <元プランの所在。path / issue URL / 「会話内」/ 「Test Inventory 報告」>   # 任意

acceptance_criteria:            # 元プランの AC を原文のまま保持する。言い換え禁止。
                                # findings 由来の AC では、原文はユーザーが確定した文言を指す
  - id: AC-1                    # 安定 ID。枝と Branch Plan の増減で振り直さない
    text: <元プランの原文>
    derived_from: []            # 任意。既定は空配列。findings 由来のときだけ元の finding ID(`G-*`)を列挙する。
                                # 空なら元プラン由来であり、findings 由来 AC と同一 Set 内で混在できる。
                                # 検査対象にしない(「設計方針」を参照)

order: []                       # branch_plans[].id を1回ずつ含む全順序。並び自体が実行順序で、
                                # 先行する Branch Plan 全体の完了を待ってから次を開始する

decision:                       # 分割しない(branch_plans が1件)場合は必須
  split: false
  reason: <1つの Branch Plan で受け入れ判断・差し戻し・テスト実行が閉じる根拠>

validation:                     # Set 帰属の violation だけを持つ
  blocking: []                  # violation の配列。1件でもあれば全 Branch Plan が status: blocked
  # - code: <violation code 表の安定 code>
  #   path: <問題があるスキーマ上の path。Set の root からの相対。
  #          例: branch_plans[1].branches[0].depends_on>
  #   message: <修正に必要な説明>

branch_plans:
  - id: BP-1                    # Set 全域で一意
    status: blocked | awaiting_review | approved
    # blocked:          「blocking violation code」の節が定める blocked の定義に従う
    # awaiting_review:  confirmation_mode: review で Set と自身に blocking なし。ユーザー承認待ち
    # approved:         承認済み。Set と自身の blocking がすべて空であることが前提
    confirmation_mode: review | auto
    # すべての status で保持する。既定は review。auto はユーザーが明示した場合のみ。
    # blocked の解消後にどちらへ遷移するかは、この値から復元する
    approval:
      method: null | user | auto  # 未承認の間は null。auto は「Branch Plan の承認」だけを
                                  # 自動化した記録であり、委譲開始権限を含まない
    delegation:                   # 承認とは独立した委譲開始権限
      authorized: false           # planning Skill は常に false で返す
      authorized_by: null | user  # ユーザーの明示的な委譲要求だけを根拠に親エージェントが設定する。
                                  # どの status でも記録できるが、委譲開始には status: approved が別途必要
      requested_mode:             # null、または配分方針と基準の2軸を持つ次の構造
        policy: fixed | adaptive
        baseline: lite | standard | strict
      # ユーザーが明示した委譲 mode。mode の明示は現行契約どおり委譲要求を兼ねるため、
      # requested_mode が非 null なら authorized: true / authorized_by: user であること。
      # mode 未指定の明示的な委譲要求は null のまま保持し、Executor が {adaptive, standard} を採用する。
      # 有効な {policy, baseline} の組み合わせは「blocking violation code」の有効な組み合わせ表で定める。
      # Executor が実際に採用した枝ごとの mode は Branch Plan へ書き戻さず、実行 Data として
      # 保持して最終報告で報告する
    unresolved_decisions:         # blocking のみ。1件でもあれば status: blocked
      - question: <確定が必要な問い>
        affects:                  # 型付き参照。kind ごとに id の必須・禁止が決まる
          - kind: branch          # id 必須。同一 Branch Plan 内の branch id の存在を検査する
            id: <branch id>
          - kind: ac-assignment   # id 必須。Set の AC id の存在を検査する。
                                  # どの枝へ割り当てるかが未確定であることを表す
            id: <AC id>
          - kind: ac-derivation   # id 必須。Set の AC id の存在を検査する。
                                  # findings から導出した AC の文言が未確定であることを表す
            id: <AC id>
          - kind: execution-order # id を持たない
        # default_assumption は持たない。仮定で進めてよい不足は assumptions へ
    assumptions:                  # minor のみ。枝構造・実行順序・AC 割り当てに影響しない仮定
      - topic: <対象>
        assumption: <置いた仮定>
        rationale: <この仮定が枝構造に影響しない理由>
    shared_foundation:            # 親が委譲前に実装する明示的な例外。委譲枝としては表現しない
      required: true | false      # false の場合、以下のフィールドは省略可
      executor: parent            # 固定値
      condition: <複数枝が共有する fixture / 設定 / テストデータ等の具体>
      forbidden_paths: []
      foundation_criteria: []     # 共有土台自身の完成条件。元 AC の言い換え禁止
      verification: []            # 基準 commit にする前の検証 command
      covers_acceptance_criteria: []  # 固定で空。元 AC の完成責任を負わないことをスキーマ上明示
    branches:
      - id: <kebab-case>                     # Set 全域で一意。cross-plan-dependency は
                                             # Set 全域の branch id で判定するため重複を許さない
        title: <短い表題>
        purpose: <外部から観測可能な振る舞い>  # 委譲 prompt の「目的」にそのまま渡せる粒度
        depends_on: []                         # 同一 Branch Plan 内の他枝の id。循環禁止
        covers_acceptance_criteria: [AC-1]     # この枝が完成責任(primary)を持つ AC。
                                               # 全 AC が Set 全域でちょうど1枝の covers に現れ、
                                               # 各枝は1件以上の AC を所有すること
        verifies_acceptance_criteria: []       # 完成責任は負わないが検証に参加する AC
                                               # (旧APIパリティの再確認など)。枝間で重複可
        branch_criteria: []                    # 枝固有の派生条件。AC の言い換え禁止
        forbidden_paths: []
        tests: [unit | integration | e2e | contract | regression]
        # 1つ以上必須。テスト種別だけを保持し、具体的なテスト名・実行 command は持たない
        # (「tests の意味」の節を参照)
        out_of_scope: []                       # この枝では担当しない責務・作業
        failure_impact:
          level: low | medium | high
          reasons: [<1件以上の具体的な理由>]
        implementation_complexity:
          level: low | medium | high
          reasons: [<1件以上の具体的な理由>]
    execution:
      order: []                   # この Branch Plan の全枝の id を1回ずつ。
                                  # depends_on の topological order であること
    delegation_mode_proposal:     # 出力条件表を満たすときだけ出力する。要否と内容は
                                  # requested_mode と枝の failure_impact.level から再計算する
      propose:
        policy: adaptive          # 引き上げの提案なので policy: fixed は提案しない
        baseline: standard | strict
      reasons: []
    decision:                     # 分割しない(branches が1枝)場合は必須
      split: false
      reason: <1枝で受け入れ判断・差し戻し・テスト実行が閉じる根拠>
    override:                     # ユーザーが分割の統合・修正を指示した場合のみ
      merge_branches: true
      reason: <ユーザーが示した理由>
    validation:                   # この Branch Plan 帰属の violation だけを持つ
      blocking: []                # violation の配列。1件でもあれば status: blocked
      # - code: <violation code 表の安定 code>
      #   path: <問題があるスキーマ上の path。この Branch Plan からの相対。
      #          例: branches[1].forbidden_paths>
      #   message: <修正に必要な説明>
      self_assessment:            # 参考情報。承認可否の判定には使わない
        action_boundaries_isolated: true      # 補助指標(第一基準ではない)
        test_boundaries_clear: true
        excessive_fragmentation: false
```

## blocking violation code

planning Skill と Executor は同じ検査規則を使う。Executor は planning Skill の自己申告を信用せず、
入力 Data から再計算する。「帰属」列は、その code をどの層の Data から再計算し、どちらの
`validation.blocking` へ入れるかを表す。

| code | 帰属 | 検査内容 |
| --- | --- | --- |
| `duplicate-id` | Set | Set 全域の Branch Plan id / branch id / AC id の重複 |
| `unknown-reference` | Set | 存在しない Branch Plan id / branch id / AC id への参照(`order`、`depends_on`、`covers_acceptance_criteria`、`verifies_acceptance_criteria`、`execution.order`、`unresolved_decisions.affects` の `branch` / `ac-assignment` / `ac-derivation`)。解決範囲は下記に従う |
| `cross-plan-dependency` | Set | 枝の `depends_on` が同一 Branch Plan 内で解決できず、Set 全域の branch id には存在する |
| `ac-unassigned` | Set | 全 Branch Plan の枝の和集合で、どの枝の `covers_acceptance_criteria` にも現れない AC |
| `ac-duplicate-primary` | Set | 全 Branch Plan の枝の和集合で、複数枝の `covers_acceptance_criteria` に現れる AC |
| `execution-order-invalid` | 両方 | Set では `order` の不足・重複、Branch Plan では `execution.order` の不足、重複、依存順序違反 |
| `branch-without-primary-ac` | Branch Plan | primary AC を1件も所有しない実装枝 |
| `dependency-cycle` | Branch Plan | 同一 Branch Plan 内の `depends_on` の循環 |
| `tests-missing` | Branch Plan | 枝の `tests` が空 |
| `branch-assessment-missing` | Branch Plan | `failure_impact` / `implementation_complexity`、または配下の `level` / `reasons` の欠落 |
| `branch-assessment-invalid` | Branch Plan | 両 field の `level` が `low` / `medium` / `high` 以外、または `reasons` が配列以外・空配列・空文字・非文字列要素を含む |
| `legacy-risk-present` | Branch Plan | 旧 `risk` が単独で存在する場合、または旧 `risk` が新しい field と混在する場合。旧 `risk` から新しい2軸を推測しない |
| `legacy-stages-present` | Branch Plan | 廃止した `implementation_stages` / `stage_tests` / `stages_reason` のいずれかが枝に存在する場合。廃止した field から枝構造を推測しない |
| `branch-contract-violation` | Branch Plan | 外部から観測可能な振る舞い単位、単独の受け入れ判断、単独 revert という実装枝契約を満たさない枝 |
| `state-invalid` | Branch Plan | `status` と他フィールドの矛盾(`approved` なのに `unresolved_decisions` が非空など)。有効な組み合わせ表から再計算する |
| `approval-invalid` | Branch Plan | `approval.method` と `status` / `confirmation_mode` の矛盾(`awaiting_review` なのに `method` が非 null、`review` なのに `auto` 承認など) |
| `delegation-invalid` | Branch Plan | `delegation` 内の矛盾(`authorized: false` なのに `authorized_by: user`、`requested_mode` が非 null なのに `authorized: false`、`requested_mode` が有効な `{policy, baseline}` の組み合わせでないなど)。有効な組み合わせ表から再計算する |
| `mode-proposal-invalid` | Branch Plan | `delegation_mode_proposal` の要否・内容が `requested_mode` と枝の `failure_impact.level` からの再計算(出力条件表)と一致しない(必要時の欠落、不要時の出力、表と異なる `{policy, baseline}` の提案) |

両評価軸で `reasons` の欠落、配列以外、空配列、空文字、非文字列要素は invalid とする。

帰属は次の基準で決める。Set 全域の Data を必要とする判定を1件でも含む code は、下記の `両方` の
条件に当たらない限り `Set` 帰属とし、同種の検査を層で割らない。層で割ると同じ違反が2層へ二重記録され、どちらを正とするかの規則が
新たに必要になるためである。AC の割り当てを Branch Plan 単体で検査すると、AC が複数の
Branch Plan へ散る正当な分割で必ず `ac-unassigned` が発火するため、全 Branch Plan の枝の和集合で
検査する。`unknown-reference` は、AC id 参照が Set の `acceptance_criteria` 全域を必要とし、
`depends_on` の未解決も `cross-plan-dependency` との切り分けに Set 全域の branch id を必要とする
ため、同一 Branch Plan 内で閉じる参照も含めて参照検査全体を `Set` 帰属にまとめる。
`両方` は、同名の検査が両層の別 field(`order` と `execution.order`)に独立して存在し、片方の
違反をもう片方から再計算できない `execution-order-invalid` だけに与える。

`unknown-reference` の検査は Set 層で行うが、参照の解決範囲は種類ごとに分ける。
AC id 参照(`covers_acceptance_criteria` / `verifies_acceptance_criteria` /
`unresolved_decisions.affects` の `ac-assignment` / `ac-derivation`)は Set の
`acceptance_criteria` 全域で解決する。branch id 参照(`depends_on` / `execution.order` /
`unresolved_decisions.affects` の `branch`)は、その参照を持つ Branch Plan 内の
`branches[].id` だけで解決する。Set の `order` の要素は `branch_plans[].id` で解決する。

`cross-plan-dependency` は、枝の `depends_on` が同一 Branch Plan 内で解決できず、かつ Set 全域の
branch id には存在する場合に生成する。Set 全域にも存在しない場合は `unknown-reference` とする。
枝間の依存が Branch Plan 内で閉じるという分割の前提が崩れている状態と、参照の書き誤りとでは
差し戻し先が異なるため、同じ code にまとめない。

Set の `validation.blocking` が非空である間、planning Skill は全 Branch Plan を `blocked` にする。
Set は `status` を持たないため、これが Set の違反を Branch Plan の状態として表す唯一の経路である。
したがって
`blocked` は、その Branch Plan の `unresolved_decisions` または `validation.blocking` が非空、
あるいは Set の `validation.blocking` が非空であることを表す。有効な組み合わせ表と `state-invalid`
の再計算は拡張後の定義を使い、自身の2 field が空のまま `blocked` である Branch Plan を矛盾として
扱わない。Set の `validation.blocking` は `state-invalid` の入力にしない。Set 由来の `blocked` は
Set 層の検査と Executor の先行検査で表し、Branch Plan 側では再判定しない。
Executor は Set 全体の検査を先に行い、非空なら Branch Plan 側の状態に関わらず実行を
開始しない。受け入れ口での再検証の規約そのものは `impl-lead` 側を正本とする。

Branch Plan の状態は値を個別に検査せず、次の有効な組み合わせ表から検査する。表にない組み合わせは
`state-invalid` / `approval-invalid` / `delegation-invalid` を生成する。

| status | approval.method | confirmation_mode |
| --- | --- | --- |
| `blocked` | `null` | `review` / `auto` |
| `awaiting_review` | `null` | `review` のみ |
| `approved` | `user` | `review` のみ |
| `approved` | `auto` | `auto` のみ |

| delegation.authorized | authorized_by | requested_mode |
| --- | --- | --- |
| `false` | `null` | `null` |
| `true` | `user` | `null`(mode 未指定の委譲要求。Executor が `{adaptive, standard}` を選ぶ) |
| `true` | `user` | `{fixed, lite}` |
| `true` | `user` | `{adaptive, standard}` |
| `true` | `user` | `{adaptive, strict}` |
| `true` | `user` | `{fixed, strict}` |

`{adaptive, lite}` と `{fixed, standard}` は入力語彙が存在しないため無効とし、表に含めない。
`baseline` を `lite` にすると low complexity 枝の割り当て先が `lite` しかなく導出が恒等写像になり、
`medium` 以上を引き上げる用途は `{adaptive, standard}` と同一になるため、独立した配分方針として
意味を持たない。`{fixed, standard}` は全枝固定を明示する入力語彙が存在しないため到達できない。
仮に語彙を足しても `{adaptive, standard}` は low complexity 枝だけを `lite` に落とし他は `standard` の
ままなので、品質面で下回らずコストだけが下がり、優位性がない。

`delegation_mode_proposal` の要否と内容は、次の出力条件表から `requested_mode` と枝の
`failure_impact.level` を使って再計算する。

| delegation.requested_mode | 枝の failure_impact.level | 出力 |
| --- | --- | --- |
| `{fixed, lite}` | `high` を含む | `{adaptive, strict}` を提案 |
| `{fixed, lite}` | `medium` を含み `high` なし | `{adaptive, standard}` を提案 |
| `{fixed, lite}` | 全枝 `low` | 出力しない |
| `{fixed, strict}` | 任意 | 出力しない |
| `{adaptive, *}` または `null` | 任意 | 出力しない |

`policy: adaptive` では枝の `implementation_complexity.level` から mode を導出する。
`failure_impact` は adaptive mode の直接導出には使わない。提案が必要なのは `{fixed, lite}` が
枝の `failure_impact` と整合しない場合だけである。

`{fixed, strict}` に対して降格を提案しない。引き上げだけを提案する非対称性は、コストの削減より
品質の担保を優先する判断であり、low failure impact 枝から `lite` を提案しないのと同じ理由である。

`branch-contract-violation` は機械検査ではなく planning Skill と Executor の判定で生成する。
`implementation_complexity.level: high` だけでは `branch-contract-violation` にしない。
単独の Acceptance Criteria・検証・受け入れ判断・revert が閉じない場合だけ、この code を生成する。
実装枝契約に関わる判定(単独 review 可能性、revert 範囲の隔離、禁止範囲の明確さ)はこの code で
表現し、`false` のまま承認へ進む経路を持たない。

## 状態遷移と権限

状態は Branch Plan ごとに持つ。

| 遷移 | 実行主体 | 条件 |
| --- | --- | --- |
| (生成) → `blocked` | planning Skill | 自身の `unresolved_decisions` または `validation.blocking`、あるいは Set の `validation.blocking` が非空 |
| (生成) → `awaiting_review` | planning Skill | `confirmation_mode: review` かつ Set と自身に blocking なし |
| (生成) → `approved` (`method: auto`) | planning Skill | `confirmation_mode: auto` かつ Set と自身に blocking なし |
| `blocked` → `awaiting_review` | planning Skill(再実行) | 原因解消後に全 validation を再実行して Set と自身に blocking なし、`confirmation_mode: review` |
| `blocked` → `approved` (`method: auto`) | planning Skill(再実行) | 同上、`confirmation_mode: auto` |
| `awaiting_review` → `approved` (`method: user`) | 親エージェント | ユーザーの承認操作。Set または自身に blocking violation が残る場合は承認操作があっても遷移しない |
| `delegation.authorized: false → true`(必要なら `requested_mode` も設定) | 親エージェント | ユーザーの明示的な委譲要求。mode の明示は委譲要求を兼ねる。どの status でも記録できるが、委譲開始には `status: approved` が別途必要 |

承認と委譲開始は独立している。`awaiting_review` から承認された場合も、委譲要求がなければ
計画の確定だけで停止する。確認モードの既定値は `review` とし、`auto` はユーザーが明示した
場合のみ使う。

## tests の意味

`tests` はテスト種別だけを保持する。具体的なテスト名、実行 command、期待値は
Branch Plan では確定しない。

- 具体的なテストは、`strict` ではテスト計画の段階で、`standard` 以下では委譲 prompt の
  AC 対応表と検証 command で確定する。いずれも親が承認・確定する現行契約に従う。
- 検証 command は対象 repository の設定に依存するため、planning Skill は command を推測しない。
- Executor は、枝の `tests` に列挙された種別が委譲 prompt の必須テストと検証 command で
  すべて充足されることを委譲前に確認する。
