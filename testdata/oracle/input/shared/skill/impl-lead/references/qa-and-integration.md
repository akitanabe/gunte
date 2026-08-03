# 返却の QA と統合

## 目次

- 返却と統合
- 親の QA
- 返却 diff の変更単位判定
- 再分割・再承認ゲート

## 返却と統合

ここでは Green / Refactor まで完了した枝の最終返却を扱う。`strict` のテスト計画、Red、Green の
中間ゲートは [実装枝の準備と委譲](implementation-branches.md) に従い、未完成の枝を統合しない。
直列受け入れは、commit 単位で返し、親が diff を読んでから1枝ずつ取り込む。

1. Implementer は worktree path、git branch、基準 commit、返却 commit SHA range、変更ファイル、
   実行した command と結果、未コミット変更を返す。
2. 親は `git -C <worktree> status --short` と `git -C <worktree> diff <base>...HEAD` を確認する。
   併せて親の checkout を `git -C <親 checkout> status --short` で確認し、worker の変更が worktree の外へ
   混入していないことを確かめる。親の統合 checkout に生成した diff artifact
   (`.tugite/diffs/<slug>-diff.patch`)は親自身が書き出した既知の untracked file であり、
   この確認によって worker の変更の混入と誤認しない。作成規約は
   [diff artifact の作成](reviewer-dispatch.md) に従う。`feature-lead` の一括経路で
   `plan-craft` が書き出したプラン artifact(`.tugite/plans/` 配下)も親側が書き出した
   既知の untracked file として同じ扱いとし、worker の変更の混入と誤認しない。作成規約は
   [プラン artifact](../../plan-craft/references/plan-artifacts.md) に従う。
3. 報告だけで受け入れず、対象 test と実装 diff を開く。
4. QA hard reject は同じ枝へ {{continuation_mechanism}} で差し戻し、修正 commit を追加させる。
5. 専門 reviewer へは task、AC、commit 範囲、変更ファイル、diff text、Branch Plan の `failure_impact.reasons`、
   親 QA が返却 diff から特定した対象リスクを渡す。
   この手順は起動可否を定めず、渡す Data と diff の受け渡しだけを定める。
   専門 reviewer の起動条件は [専門 reviewer](reviewer-dispatch.md) に従う。
   周辺コンテキストの追加は [reviewer へ渡すコンテキスト](reviewer-dispatch.md) の選択基準に従う。
   {{new_worker}} は別 worktree で始まり枝の変更を見ないため、作業 tree の存在を前提にさせない。
   ここでの作業 tree は worker worktree を指し、親の統合 checkout に保存した diff artifact を
   reviewer が Read することとは矛盾しない。diff の受け渡しは
   [reviewer 起動テンプレート](reviewer-dispatch.md) に従い、
   diff artifact の絶対 path を渡すことを既定とし、artifact を生成できない場合だけ diff text 欄へ
   本文を直接記入する。
6. 受け入れ後は統合先の git branch で `git cherry-pick <sha>` または commit range を取り込み、focused test と
   関連する build、typecheck、lint を再実行する。
7. 統合後の green commit を次の枝の基準にする。枝 worktree の green と統合後の green の片方を省略しない。

### 部分成果の受入と context replacement

Implementer の返却に含まれる未完成 production code は統合しない。親が独立に受入可能と QA した成果だけを、
この節の「返却と統合」の手順で受け入れる。部分成果は承認済み purpose / AC / scope を変えない。単独で green にできる
commit range に限る。返却 diff の変更単位判定と再分割・再承認ゲートを先に通す。部分成果の受入判断と QA を行い、条件不成立なら何も統合せず、
元の green 基準から新しい context を作成する。条件成立後は focused test と関連検証を通した基準 commit にする。
再委譲では、部分成果の受入判断と QA、基準 commit 検証、旧 context の worktree / git branch 破棄、基準 commit から新しい
Implementer context を作成し、その worktree / git branch を準備する順に進める。これは run-closeout の最終 cleanup ではない。
## 親の QA

`standard` と `strict` では全観点を手を動かして確認する。`lite` では観点0（diff を読む）、観点5
（自分で green を確認）、Acceptance Criteria に対応する振る舞いが検証されていることの確認へ絞ってよい。
`lite` のこの確認は親が diff と検証結果から行うものであり、Implementer への AC 対応表や
Red 証跡の要求に置き換えない。`lite` の前提が崩れた場合は mode を引き上げる。

委譲 mode によらず、次を判定原則とする。

- 検証手段はテストに限定せず、プロジェクトまたはタスクで指定された成功条件（自動テスト、type check、
  lint、build、静的解析、実行結果の確認、手動確認手順、snapshot 比較、API レスポンス確認など）を使う。
- 検証 command が成功したことだけを完了根拠にしない。
- 親は「どの Acceptance Criteria を」「どのテストまたは確認手順で」「どの結果によって」満たしたと
  判断したかを説明できる状態にする。

0. **実装 diff** — 基準 commit からの diff を開き、`forbidden_paths` に列挙された変更禁止範囲へ
   接触していないことに加えて、枝の `out_of_scope` に列挙された責務・作業を含まないことを確認する。
   既存設計からの逸脱、公開契約の破壊、
   既存 test の弱体化、未承認依存、error handling、resource 解放、concurrency、security も確認する。
1. **振る舞い** — test が private API や実装手順ではなく、外部から観測可能な振る舞いを検証しているか。
2. **網羅性** — AC、境界値、異常系、例外経路、分岐、期待値の根拠が実際の test と一致するか。
3. **TDD** — 新機能または未実装仕様なら Red 出力または段階 commit を確認し、test を実装へ合わせて
   弱めていないか。既存挙動を固定する regression test が追加時点で Green なら、親は AC、test、期待値の
   根拠、既存挙動の対応を実際の test と実装から確認し、既存実装がすでに仕様を満たすという返却根拠が
   妥当か判断する。形式的な Red のための本番 code 変更がなく、mutation を使った場合は親が明示した
   一時検証だけであること、mutation が commit されておらず、変更禁止範囲や本番 code に接触していない
   ことも確認する。
4. **記述原則** — Code=How、test=What、commit=Why、comment=Why Not の配置になっているか。
5. **親の実行** — focused test と関連する全体検証を親が実行し、green を確認する。

`failure_impact.reasons` に記録された失敗伝播、部分成功、rollback 影響を確認する。
rollback の確認を `implementation_complexity` から導出しない。
## 返却 diff の変更単位判定

親は、返却 diff を専門 reviewer 起動や受入の前に読み、1変更単位として渡せるかを判定する。判定は
固定行数やファイル数ではなく、変更理由・Acceptance Criteria (AC)・責務・依存・受入・rollback・検証単位を
対応付けて行う。次のいずれかがある場合は、混在した diff のまま reviewer へ渡したり受け入れたりせず、
「再分割・再承認ゲート」へ進む。

- 独立した変更理由、AC 無関係変更、または異なる責務・依存が混ざっている。
- 異なる rollback・review・前提知識・責務・受入単位・検証単位があり、reviewer が一変更単位として判断しにくい。
- 計画 scope の大幅超過、または承認済み scope と目的の対応を説明できない。

これらが混在し reviewer が一変更単位として判断困難な場合も、同じ停止条件を適用する。

diff が大きいだけでは分割しない。固定行数だけでは分割しない。分割すると依存が不自然になったり、外部から観測可能な検証が成立しなく
なったりする場合は、複数の層や作業種別を無理に枝へ分けず、1変更として扱い、必要な reviewer context と
検証単位を明示する。
## 再分割・再承認ゲート

混在 diff をそのまま reviewer へ渡したり受け入れたりしない。親は次のうち、承認済み契約を保つ整形、
差戻し、または再計画のいずれかを選び、選択理由を記録する。

- scope 逸脱の差戻しを選ぶ。
- 承認済み実装枝の purpose、AC 文言、AC ownership、scope、依存、failure impact、implementation complexity を保てる場合は、commit を分離する、
  最小範囲だけを残す、または別タスク化する。この整形では、既存枝の契約と承認を変更しない。
- 独立した実装枝への分離、または AC ownership・依存・failure impact・implementation complexity の変更が必要な場合は Branch Plan を再生成する。
  blocking violation と Executor 再検証5項目を再計算し、必要なユーザー再承認を得るまで新枝を委譲しない。
- AC 文言自体の分解・再定義が必要な場合はプラン文書の AC 確定とユーザー確認へ戻る。その後
  Branch Plan を再生成・再検証・再承認する。再承認後に初めて新枝の委譲を開始する。

分割で依存が不自然または検証不能になる場合は1変更として扱う。この場合も、固定行数を根拠に分割を強制せず、
親が reviewer 起動前に判断を記録する。承認済み契約を保つ整形と、契約・AC の再確定を要する再計画を混同しない。
