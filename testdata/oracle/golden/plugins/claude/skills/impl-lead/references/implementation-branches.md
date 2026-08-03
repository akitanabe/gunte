<!-- Generated from shared/. Do not edit directly. -->

# 実装枝の準備と委譲

## 目次

- 用語
- Implementer context と枝の lifecycle
- worktree と基準 commit
- 委譲 mode に応じた TDD/QA
- Implementer の選択
- 委譲 prompt

## 用語

- **実装枝** — 委譲単位。外部から観測可能な振る舞いの縦割りで、単独の Acceptance Criteria、
  検証、受け入れ判断、revert が可能な大きさを持つ。Branch Plan の `branches[]` 1件に対応する。
  文脈が実装枝に限定される箇所では「枝」と略す。
- **git branch** — 実装枝を載せる VCS 上の branch。1実装枝 = 1 git branch = 1 専用 worktree で
  対応させる。日本語文中では常に `git branch` と書き、単独の `branch` 表記を使わない。
  実装枝と同じ文に現れるため、修飾なしでは指示対象が確定しないため。
- **Branch Plan Set** — `branch-design` が出力する Data。`branch_plans[]` に Branch Plan を持ち、
  `acceptance_criteria` と `order` を Set 層で持つ。
- **Branch Plan** — Branch Plan Set の要素。`branches[]`、
  `branch_criteria`、`branch-without-primary-ac` などの key と code は実装枝を指し、
  git branch を指さない。

## Implementer context と枝の lifecycle

上位ルールは **1実装枝 = 1つの新規 Implementer context** とする。実装枝は外部から観測可能な振る舞いを
単位として分け、単独の Acceptance Criteria、検証、受け入れ判断、revert が可能な大きさにする。
各実装枝を開始するときは新しい Implementer を生成し、別の実装枝に同じ Implementer を再利用しない。

同じ Implementer を継続できるのは、同一実装枝を完成させるための段階ゲートと差し戻しに限る。
Acceptance Criteria 未達、仕様誤解、機能欠落、テスト失敗、正常系・異常系・境界値不足、スコープ逸脱、
再検証、`strict` mode の Red / Green / Refactor は、同じ context と worktree で継続する。
これは同じ実装枝の段階ゲートと QA 差し戻しを指す。Implementer の返却時に親が設計・枝粒度・worker 適性を
再判断する場合は、次段落の新しい context へ切り替える。

Implementer の返却で設計の未確定、枝の粒度、worker の適性に関する判断点が残った場合は、親が
「設計を確定して `implementer` に再委譲」「枝を追加分割」「senior へ再配車」のいずれかを選ぶ。
この再配車は段階ゲートの継続ではなく、新しい routing snapshot を作る別イベントであり、新しい Implementer context
で行う。未完成 production code は統合せず、親が独立に受入可能と QA した成果だけを
[返却と統合](qa-and-integration.md) の手順で受け入れる。部分成果は承認済み purpose / AC / scope を変えない。単独で green にできる
commit range に限る。返却 diff の変更単位判定と再分割・再承認ゲートを先に通す。部分成果の受入判断と QA を行い、条件不成立なら何も統合せず、元の green 基準から新しい context を作成する。
条件成立後の順序は、部分成果の受入判断と QA、基準 commit の検証、旧 context の worktree / git branch の破棄、
基準 commit からの新 context の worktree と git branch の作成とする。
旧 context の破棄は run-closeout の最終 cleanup ではなく、再委譲に先立つ context replacement である。
返却された状況と判断点は確定済み設計判断として新しい prompt に載せる。
「1実装枝 = 1つの新規 Implementer context」は枝をまたぐ再利用を禁止する規約であり、同一枝の破棄・
新 context 再開は禁止しない。

枝を統合し、統合後の green を確認して差し戻しが不要になった時点で、その Implementer の役割を終了する。
次の枝は最新の統合済み green な基準コミットから開始する。前の枝から引き継ぐ変更は統合済みコードへ
反映し、コードから読み取れない確定済み制約だけを次の指示へ明記する。

この規約は Branch Plan 間にも適用する。次の Branch Plan の先頭枝も、先行 Branch Plan の成果を
含む最新の統合済み green な基準コミットから開始する。
先行 Branch Plan の成果が main へ merge されるのを待たない。merge を待つと、人間の merge 操作が
workflow の待ち合わせに入り、`impl-lead` が完了を判定できない外部要因に実行順序が依存する。

実装枝を開始するときは新規の `Agent` 呼び出しで Implementer を生成する。同一枝の段階ゲートと
差し戻しには `SendMessage` を使い、別の枝へ進むために同じ Agent を継続しない。

## worktree と基準 commit

各実装枝は専用 worktree で隔離する。worktree を用意できない場合は委譲を開始しない。
worktree の目的は並列速度ではなく、枝ごとの diff、検証、差し戻し、revert を独立させることにある。

親が最新の基準 commit から実装枝専用の worktree と git branch を作成し、絶対 worktree path、
git branch、基準 commit を Implementer へ渡す。Implementer はファイル変更前に、作業場所が指定
worktree であること(`pwd -P`)、git branch 一致、HEAD が基準 commit と一致すること、
`git status --short` が空であることの開始条件4点を確認し、1つでも不成立なら何も変更せず親へ返す。
reset / merge / checkout などで自力修復しない。
worker は指定 worktree の外を編集しない。cleanup は親が `git worktree remove` で行う。

`Agent` 起動時に cwd を直接指定できないため、worktree 隔離を `Agent` 側の option に任せず、委譲 prompt の
絶対 path 指示で worktree へ到達させる。親は worktree 配置先へ subagent の書き込みが permission 上到達できる
ことを事前に確認する。

- 共有 fixture、設定、ロックファイル、テストデータ、自動生成物などを複数枝が必要とする場合は、
  親が共有土台として先に確定し、検証済みの基準 commit にする。
- DB、Redis、queue、port、共有 temp、生成 cache、`.env`、外部 API mock は worktree では隔離されない。
  枝専用 resource を割り当てるか、直列実行で衝突を避ける。
- 委譲直前に基準 commit で既存 test、build、typecheck、lint を実行し、green を確認する。
- 基準が red の場合は委譲を開始せず、既存失敗として切り分ける。

## 委譲 mode に応じた TDD/QA

表の `委譲 mode` は枝ごとに導出された枝 mode であり、配分方針 `{policy, baseline}` ではない。
導出は [Branch Plan の受け入れ](branch-plan-intake.md) の「枝 mode の決定表」に従う。

| 委譲 mode | TDD/QA の強度 |
| --- | --- |
| `lite` | 親は返却の diff とテストを確認し、Acceptance Criteria に対応する振る舞いが検証されていることを確かめ、focused test またはタスクで指定された成功条件で green を確認する。段階ゲート、AC 対応表、Red 証跡は親が明示した場合だけ要求する。 |
| `standard` | AC→テスト対応表、境界値、異常系、Red 時点の失敗出力を要求する。返却物を QA ルーブリックの全観点で精査し、親が green を確認する。 |
| `strict` | テスト計画→失敗テスト→実装→Refactor の段階ゲートに分ける。各段階を親が確認し、最終返却物には `standard` と同じ QA を行う。 |

全ての委譲 mode で、親による統合後の検証と最終的な受け入れ判断を省略しない。

### Red 証跡と regression Green 例外

`standard` と `strict` では、新機能または未実装仕様を検証する test は Red 必須とし、失敗出力または
段階 commit で未実装時の失敗を確認する。この要件を、test 追加時にすでに存在する振る舞いを固定する
ための作業へ形式的に適用しない。既存挙動を固定する regression test に限り、次の根拠を返却物へ含める
ことで追加時点で Green であることを許可する。

- 既存挙動を固定する追補 test であること
- 対応する AC
- 期待値の根拠
- 既存実装がすでに仕様を満たしていたこと

これは「最初から Green ならよい」という一般例外ではない。`strict` でも Test plan / Red / Green /
Refactor の段階順序を維持し、Red gate で上記の Green 結果と根拠を確認する。既存実装がすでに AC を
満たすため Green 実装が不要な段階では、空 commit を要求しない。形式的な Red を作るために本番 code を
一時変更してはならない。

mutation は親が明示した一時検証に限定する。親は対象、方法、復元確認、検証 command を明示し、mutation
を commit してはならない。変更禁止範囲や本番 code を mutation の対象にしてはならない。検証後は変更が
残っていないことを親と Implementer の双方が確認する。

`strict` は同じ Implementer と worktree を次の段階で継続する。

1. **テスト計画** — どの AC、境界、異常系をどう検証するかだけを返させ、親が承認する。
2. **失敗テスト** — 新機能または未実装仕様では、実装せず、狙いどおり fail するテストと失敗出力だけを
   返させ、親が確認する。regression Green 例外では、追加時点で Green の test と上記4項目を返させる。
3. **Green の実装** — 最小実装で test を通し、後から期待値を実装へ合わせていないか親が確認する。
4. **Refactor と再検証** — 振る舞いを保った整理、focused test、必要な全体検証を行い、親が最終 diff を確認する。

テスト計画では commit を作らない。Red、Green、Refactor の各段階では、段階の変更を commit する。
新機能または未実装仕様の Red commit は failing test を含むため統合せず、同じ worktree で次段階へ進める。
regression Green 例外の Red 段階では passing test を commit し、変更がない Green / Refactor 段階に空 commit
を作らない。各段階の返答には、その段階に commit があれば SHA と検証結果を含め、最終返却では先頭から
末尾までの commit SHA range を返す。段階の commit 後に未コミット変更を残さない。

段階ゲートを使う枝は、同じサブエージェント（同じ worktree）を `SendMessage` で継続する。
新規の `Agent` 呼び出しは別 context になるため、同一枝の途中で切り替えない。継続不能の場合は、
親が受け入れ済みの失敗テストをその枝へ commit してから次フェーズを委譲する。

## Implementer の選択

難度は `implementation_complexity` と実装時に残る設計・推論判断で判断する。

### 候補抽出と実割当

`implementation_complexity` は Branch Plan の mode 導出に使う入力であり、senior 候補は Branch Plan の field にせず、
`impl-lead` 内部の作業 Data として保持する。候補抽出と実割当を分離する。候補 Data には次の共通軸を記録する。

- 事前設計後も残る判断量
- 推論難度
- 誤実装時の手戻り量
- 他枝への影響

変更量やファイル数だけを昇格根拠にしない。現在授権され、5項目の再検証と mode 導出を通過した実行対象
Branch Plan 1件の全枝を同一の受入 snapshot 内で評価し、候補抽出後に Branch Plan 単位で実割当を一括確定する。
未授権の後続 Branch Plan を配車母集団に含めない。同一の受入 snapshot 内で候補と配車を揺らさない。
Implementer の返却は、返却 Data を含む新しい routing snapshot を作る別イベントであり、初回配車の固定を
その snapshot へ自動継続しない。

senior 候補同士を相対比較し、判断密度の高い枝から配分する。senior 候補が全枝の過半になった場合は、
枝分割または Acceptance Criteria の粒度を見直すシグナルとして扱う。固定的な割合や閾値を senior 昇格の根拠にしない。
各枝の比較結果を記録する。

senior の割当理由には、次の3点を必ず記録する。

1. 残存設計判断
2. 上位 model で減らせる誤実装・手戻り
3. 他候補より優先する理由

#### Why Not: senior の事前 reviewer

expert と異なり senior には事前 reviewer を挟まず、親の自己申告に留める。senior は候補抽出時の相対比較と
3点の理由記録で配車の根拠を明示でき、expert のように親相当の能力を独立審査する高コストな gate を必要としないためである。
expert を候補にする場合だけ [Expert 選択](expert-selection.md) の事前審査を使い、この区別を senior の選択へ持ち込まない。

| Implementer | 使う場面 |
| --- | --- |
| `implementer` | senior 候補に該当せず、仕様が明確で既存 pattern を適用でき、判断密度が低い枝。 |
| `senior-implementer` | 共通4軸の相対比較で判断密度が高く、残存設計判断と上位 model で減らせる手戻りが他候補より大きい枝。 |
| `expert-implementer` | 親相当の推論が必要で、senior では不足する具体的根拠があり、事前審査を通過した枝。 |

単なる複数 module への波及、高い失敗コスト、誤実装の代償だけでは `senior-implementer` を選ばない。
通常と senior で迷ったら `implementer` を選ぶ。迷いだけでは senior 候補にしない。難所と定型作業が混在する場合は枝を分ける。
expert と迷う場合は senior を選び、expert 候補にする場合だけ
[Expert 選択](expert-selection.md) の事前審査を行う。


## 委譲 prompt

新規 Implementer は親や前の枝の context を持たない前提にする。次の Data を自己完結して渡す。

- 実装枝の目的
- Acceptance Criteria
- 変更を禁止する物理的範囲、この枝でやらないこと
- 最新の基準コミット
- 絶対 worktree path と git branch 名
- コードから読み取れない確定済みの設計判断や制約
- 委譲 mode と TDD 要件
- 検証 command
- 完了条件
- commit と返却報告の形式

custom agent の developer instructions にある安定契約を長く再掲せず、タスク固有の Data を中心にする。

```text
## タスク
- 目的: <外部から観測可能な振る舞い>
- 実装内容: <実現する振る舞い>

## 実行 context
- 委譲 mode: <この枝に導出された枝 mode。lite / standard / strict>
- 現在の段階: <一括実装 / test plan / Red / Green / Refactor>
- 最新の基準コミット: <green を確認した SHA>
- 絶対 worktree path と git branch 名: <path / git branch>
- 確定済みの設計判断と制約: <なければ「なし」>
- 検証 command: <focused test / build / typecheck / lint>

## 受け入れ条件
- AC-1: <条件>
- AC-2: <条件>

## scope
- 変更を禁止する物理的範囲: <forbidden_paths>
- この枝でやらないこと: <out_of_scope。空なら「なし」>
- 再利用する共有基盤: <fixture / helper / test data>
- 最低限の境界値・異常系: <具体列挙>

## タスク固有の制約
- <この枝だけに追加する禁止事項や外部制約。なければ「なし」>

## 段階の返却条件
- commit: <不要 / Red commit / Green commit / Refactor commit / 最終 commit SHA range>
- 証跡: <この mode と段階で必要な AC 対応表、Red 出力、検証結果>
```

`forbidden_paths` は変更禁止の物理的範囲、`out_of_scope` は担当しない責務・作業を表す。
`out_of_scope` はパス制約へ統合せず、各項目の意味を変えずに「この枝でやらないこと」へ列挙する。
Implementer は、その責務・作業が枝の完成に必要になった場合は変更せず、必要性と理由を親へ報告する。

委譲 prompt の「委譲 mode」欄には、その枝に導出された枝 mode を書き、配分方針 `{policy, baseline}` を
渡さない。Implementer は枝 mode とその枝で要求される TDD 要件だけで作業でき、配分方針を知る必要が
ないためである。

`lite` では、親が明示した場合だけ AC 対応表と Red 時点の失敗出力を付けること。
`standard` では、Red 時点の失敗出力と
「AC-n → それを検証するテスト名 → 期待値の根拠（仕様のどこから導いたか）」の対応表を必ず付けること。
`strict` の途中段階では、その段階で要求した成果物だけを返させ、最終返却には `standard` と同じ
AC 対応表と Red 証跡を含める。regression Green 例外では、Red 時点の失敗出力に代えて上記4項目と
追加時点の Green 結果を付ける。

role profile で代替する場合だけ、担当難度、仕様を広げないこと、指定された段階を越えないこと、
既存 test の弱体化と未承認依存を禁止すること、Code=How / test=What / commit=Why /
comment=Why Not、段階別 commit と返却 schema を短く補う。

最も価値があるのは、親が境界値・異常系を具体化することである。ここを Implementer へ丸投げしない。
