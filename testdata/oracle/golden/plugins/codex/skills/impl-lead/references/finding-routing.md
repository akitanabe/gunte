<!-- Generated from shared/. Do not edit directly. -->

# Finding の修正 routing

## 目次

- evidence を欠く指摘の扱い
- 過剰実装ゲートの除去許可
- 修正先の選択
- 責務境界

## evidence を欠く指摘の扱い

reviewer が示す evidence は [Reviewer findings の共通契約](reviewer-findings.md) の
「指摘ごとの evidence」に従う。
evidence を欠く指摘は、単独でゲート通過の根拠にしない。

親が該当ファイルと行の引用・再現手順・参照した Data の path と id のいずれかを、自分が読んだ diff・
テスト結果・repository の現状から特定できる場合は、親が evidence を補って通常の判断へ戻す。
この扱いは [必須完了ゲート](branch-review.md) の reviewer に限らず、
[専門 reviewer](reviewer-dispatch.md) の reviewer を含む指摘全般に適用する。
## 過剰実装ゲートの除去許可

除去を許可する場合、親は指摘IDごとに次をすべて確認する。

- 除去後も対象 AC を満たす実装と検証が残ること
- 除去しても外部から観測可能な振る舞いと公開契約が変わらないこと
- 除去する操作が局所的で、周辺の再設計を必要としないこと

1つでも満たさない場合は元 Implementer へ差し戻す。

除去後も対象 AC を検証する要素が残るかを親が判定できず、1つ目の条件をそもそも確認できないため、
類型 C（残る検証を特定できないテスト）は `review-patch-refactorer` へ渡さず、元 Implementer へ差し戻す。

重複テストの除去では、削除する側と残す側をファイルとテスト名で特定し、起動 prompt へ明示する。
個別許可のない除去を行わせない。
## 修正先の選択

すべての reviewer の finding は、修正先または不採用のいずれを判断するより前に、親が重要度を確定する。
この適用範囲とタイミングは、以下に列挙する起動条件など本節内の他の記述の適用有無に関わらず成立する。

親は reviewer が申告した重要度をそのまま採用しない。親が確定する根拠は、finding の evidence と、その
finding が影響する Acceptance Criteria・対象 risk への影響である。reviewer 原稿が `軽微` / `修正推奨` /
`修正必須` の意味を定めている範囲については、その記述を正本として照合する。現状、`impl-lead` が起動する
reviewer に限れば、この範囲は `responsibility-boundary-reviewer` の「修正コストに見合わない指摘は `軽微`
として扱う」だけである。`Pass` / `Needs
attention` / `Blocker` のように3区分と異なる語彙で申告する reviewer の finding は、申告語彙から3区分への
写像規則を定義しないため、evidence と AC・対象 risk への影響だけから確定する。判定区分に相当する項目を
持たない reviewer の finding も同じ扱いとする。

親が確定する値は、重要度に相当する項目を持たない finding を含め、常に `軽微` / `修正推奨` / `修正必須` の
いずれかとする。原稿の他の記述が `軽微` / `修正推奨` / `修正必須` で分岐する場合、その分岐は reviewer の
申告値ではなく親が確定したこの値を指す。「責務境界」節の routing はこれに当たる。

次の条件をすべて満たす場合に限り `review-patch-refactorer` を起動する。

- 専門 reviewer（必須完了ゲートの reviewer を含む）の具体的な指摘が存在する。
- 親が指摘を確認し、修正対象として採用している。
- Acceptance Criteria は満たされている。
- Acceptance Criteria を変更する必要がない。
- 機能的なテストは green である。
- 修正範囲が局所的である。
- 仕様の再解釈を必要としない。
- 新機能追加ではない。
- 振る舞いを維持したまま修正できる。
- reviewer が修正方針または問題箇所を明示している。
- evidence を欠く指摘は、「evidence を欠く指摘の扱い」に従い、親が evidence を補って
  通常の判断へ戻している。

起動 prompt には少なくとも次の Data を含める。

- 指摘元 reviewer、対象となる指摘ID、指摘本文
- 親が採用した修正条件
- 対象 worktree、git branch、基準 commit、対象 commit 範囲
- Acceptance Criteria
- 親が個別に許可した修正範囲と変更を禁止するファイル
- 削除・移動・新規作成の可否と commit の要否
- 必須検証 command
- 除去を許可する場合の、指摘IDごとの除去対象と残す対象。pass-through 層の除去では、付け替えが必要な
  呼び出し箇所のファイルを親が個別に許可した修正範囲へ含める

入力が不足する場合、`review-patch-refactorer` は推測で補わず、ファイルを変更せず親へ返す。

`review-patch-refactorer` は指摘範囲だけを修正し、新しい問題を探索しない。仕様変更、ついで修正、
大規模再設計、新規依存追加、通常実装の代行をさせない。親が個別に許可しない限り、ファイルの
新規作成・削除・移動、指摘外のテストケース追加、テストケースの削除、fixture や helper の追加をさせない。

返却後、親は自己申告だけを信用せず、次を再確認する。

- `git -C <worktree> status --short`
- 基準 commit からの変更ファイル一覧と diff
- 親が個別に許可した修正範囲外の変更がないこと
- 親が個別に許可していないファイルの追加・削除・移動がないこと
- reviewer 指摘外の変更がないこと
- 親が個別に許可していないテストケースの削除がないこと
- テストケースの追加・変更、期待値、skip 設定の変更がないこと
- 除去を許可した場合は、除去対象が許可した指摘IDと一致し、対象 AC を満たす実装と検証が残っていること
- Acceptance Criteria と外部から観測可能な振る舞いが維持されていること
- focused test と関連する全体検証が green であること

次は元 Implementer へ差し戻す。

- Acceptance Criteria 未達
- 仕様誤解
- 機能欠落
- テスト失敗
- 正常系・異常系・境界値不足
- security や副作用の修正に振る舞い変更が必要
- test 品質の修正にケース追加や期待値の再検討が必要
- 過剰要素の除去に仕様判断、AC の再解釈、振る舞い変更が必要
- 失う AC が特定できないテストの除去
- `strict` mode の Red / Green / Refactor 継続
- 元の調査・実装判断が必要

親がその場で直してよいのは、返却後の import 整理と formatter 適用だけとする。共有土台の作成は
委譲前の明示的な例外であり、返却後の仕様判断、case 追加、命名、comment、test 名、設計修正を親が
引き取る理由にはしない。
## 責務境界

返却物 QA を通過した diff は、最終検証前に親が次を軽量確認する。

- 1つの function、class、module に複数の変更理由が混ざっていないか。
- input validation、業務判断、永続化、外部 I/O、表示整形が同じ場所に詰め込まれていないか。
- DB、API、HTTP、file、framework の具体実装を上位層が知りすぎていないか。
- 副作用が分散し、再実行、test、失敗時の扱いが難しくなっていないか。
- boolean flag や mode 引数で大きく責務を切り替えていないか。
- 既存の責務配置、命名、directory 構成から不自然に外れていないか。
- 分割や抽象化が過剰になっていないか。

この軽量確認で、親 QA が返却 diff から特定した対象リスクとして
`responsibility-boundary-reviewer` の対象リスクが成立したと判断した場合は、
[専門 reviewer](reviewer-dispatch.md) の起動条件に従って起動する。対象リスクが成立する具体例も
[専門 reviewer](reviewer-dispatch.md) が定める。この節は専門 reviewer の起動条件を
独自に定義しない。以下は起動した場合の判定区分ごとの修正先 routing である。

- `問題なし`: 通過。
- `軽微` / `修正推奨`: 局所的で全起動条件を満たす場合だけ `review-patch-refactorer`、それ以外は元 Implementer。
- `修正必須`: 解消するまで完了しない。振る舞い変更や AC 再解釈が必要なら元 Implementer。

`responsibility-boundary-reviewer` は修正しない。新しい reviewer worker として diff text を渡し、
全体判定と、指摘ごとの問題箇所、種類、理由、影響範囲、最小修正方針を返させる。
diff にない既存問題は「既存課題」として判定から分ける。
起動時は[reviewer 起動テンプレート](reviewer-dispatch.md)の全欄を埋め、diff artifact の絶対 path を渡すことを既定とする。
