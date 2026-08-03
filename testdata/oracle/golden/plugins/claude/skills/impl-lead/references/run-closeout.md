<!-- Generated from shared/. Do not edit directly. -->

# Run の終了処理

## 目次

- Branch Plan 単位の終了処理
- 未統合で終了する場合
- 統合済み diff review
- 後始末
- 最終報告

## Branch Plan 単位の終了処理

Run の終了処理(統合済み diff review、後始末、最終報告、テスト一覧 file、永続 QA レポート)は
Branch Plan 単位で行う。
未実行の後続 Branch Plan があっても、完了した Branch Plan の後始末と最終報告は行う。
境界で止まった状態を「未完了の run」として保留にすると、Branch Plan を承認・実行の単位にした
意味がなくなる。永続 QA レポートの生成単位と生成時点は [永続 QA レポート](qa-report.md) を
正本とし、本節は他の終了処理と同じ単位で行うことだけを述べる。

- 完了した Branch Plan の id は実行 Data として親が保持し、Branch Plan Set へ書き戻さない。
  `status` に完了を表す値を足さない。導出した枝 mode を書き戻さない現行契約と同じ扱いであり、
  値域・有効な組み合わせ表・状態遷移表・下流の差し戻し規則のすべてへ新しい状態を通さずに済む。
- 境界で止まった後に再開する場合、完了済みの Branch Plan を再実行しない。
  実行 Data を復元できない場合は、Branch Plan ごとの最終報告と統合済み commit を根拠に
  親が完了済みを確定してから再開する。run の途中で Set を再生成した場合も、保持していた
  完了 id を破棄して同じ根拠から完了範囲を確定し直す。再生成した Set の id と `order` は
  元の Set と対応しないため、保持していた id をそのまま突き合わせられない。
- 最終報告は Branch Plan ごとに作り、`order` 順に並べる。Set 全体の新しい要約は作らない。
- テスト一覧 file は Branch Plan ごとに生成し、名前の衝突は既存の suffix 選択規約で解決する。
  Branch Plan 単位の新しい命名規約は作らない。
- 先行 Branch Plan が完了せずに終了した場合(親が未統合の枝について `Rejected` / `Needs revision` を
  最終判断とした場合)は、後続 Branch Plan を実行せず未実行として報告する。`order` は先行
  Branch Plan 全体の完了を依存とするため、完了していない基準 commit の上で後続を実行すると
  依存の定義に反する。`unattended` で全 Branch Plan が授権済みの場合も、この規定が授権より優先する。

## 未統合で終了する場合

通常の `Needs revision` は [修正先の選択](finding-routing.md) へ差し戻し、top-level workflow を継続する。
親が未統合の枝について `Rejected` / `Needs revision` を最終判断とし、
top-level workflow を終了する場合だけ、実行可能な検証を行い、未実行の検証、未統合の理由、
worktree を保持する理由を Data として記録し、
main の手順9へ戻る。
## 統合済み diff review

全枝の統合と検証後、後始末より前に統合済み diff を review する。
review 対象には、残存する failure impact と返却 diff 由来の対象リスクを含める。

親が統合済み diff と test を読み、最終受け入れ判断を記録する。
## 後始末

後始末は、受け入れ判断、最終検証、必要な統合済み diff review を含む最終ゲートをすべて通過した後にだけ行う。
差し戻しまたは再検証の可能性がある間は始めない。

親がこのタスク用に作成した、統合済みで未コミット変更のない worktree を `git worktree remove <worktree path>`
で削除する。削除できない worktree は理由と残った path を最終報告に含める。

この run に生成した diff artifact を[diff artifact の削除](reviewer-dispatch.md)の手順で削除する。永続 QA レポートと
`<slug>-tests.md` は削除しない。
## 最終報告

- 変更内容
- Implementer が検証したこと
- 統合後に親が検証したこと
- 統合済み diff review で確認したこと
- 追加・変更したテストの一覧
- 未検証の残り

追加・変更したテストの一覧は、追加されたテストを全て読まなくても何が検証されたかを確認できる
ように、次の表で提示する。

| テスト名 | 対象 | 分類 | 検証している振る舞い | 対応 AC |
| --- | --- | --- | --- | --- |

- 行は workflow が追加・変更したテストに限り、スイート全体は走査しない。既存スイートの棚卸しは
  `test-audit` skill の責務であり、この表は語彙(対象・分類・検証している振る舞い)だけを
  揃える。
- 親が QA で読んだ diff から記入する。分類は `正常系` / `境界値` / `異常系` とし、判別できない
  場合はその旨を記す。
- 対象テストがない run は表を出さず `該当なし` と明記する。

会話上の提示に加えて、同じ表を Markdown file としても保存する。会話ログが流れても一覧を後から
参照できるようにするためであり、永続 QA レポートと異なり既定で生成する。

- 保存先は `.tugite/reports/<slug>-tests.md` とする。slug の正規化、衝突時の suffix 選択、
  安全な作成 Action、untrusted field の sanitize、Git 管理と保持は
  [永続 QA レポート](qa-report.md) の規約を正本として同じ手順に従う。衝突時の suffix は
  `-tests` を保持したまま `<slug>-tests-2.md` の順に選ぶ。
- file には見出しと上の表と同じ Data だけを記入し、それ以外の証跡を含めない。
- 対象テストがない run は file を生成しない。
- sanitize できない、または安全な作成 Action を保証できない場合は生成せず、理由を会話上の
  最終報告へ含める。
