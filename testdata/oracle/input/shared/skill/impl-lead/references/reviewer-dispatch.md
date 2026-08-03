# Reviewer の起動と diff の受け渡し

## 目次

- 専門 reviewer
- reviewer 起動テンプレート
- reviewer 起動前後の worktree・親 checkout 照合
- diff artifact の作成
- diff artifact の受け渡しと停止条件
- diff artifact の削除

## 専門 reviewer

専門 reviewer は、Branch Plan に記録された失敗影響または返却 diff で判明した reviewer 固有の対象リスクを
深く確認する役割であり、専門 reviewer を汎用コードレビューの代替にしない。
専門 reviewer は mode 名だけを理由に一律起動しない。次のいずれかが成立する場合だけ使用する。

- ユーザーが専門 reviewer を明示的に要求した場合。
- 親が reviewer の責務と一致する具体的な `failure_impact.reasons` を特定した場合。
- 親 QA が返却 diff から reviewer の責務と一致する具体的な対象リスクを特定した場合。

| Reviewer | 対象リスク |
| --- | --- |
| `responsibility-boundary-reviewer` | 責務混在、設計境界、分散した副作用 |
| `test-quality-reviewer` | 弱いテスト、欠けているケース、実装詳細に依存したテスト |
| `security-side-effect-reviewer` | 外部 I/O、破壊的操作、機密データ、セキュリティ影響 |

`responsibility-boundary-reviewer` の対象リスクは、複数層、複数の外部 I/O、新しい abstraction・adapter・service、
責務混在の疑いのいずれかを認めたときに成立する。

Branch Plan の `failure_impact.reasons` は安全性・失敗影響の事前 Data であり、返却 diff 由来の対象リスクは
親 QA が実装結果から特定する Data である。前者は起動の有無にかかわらず reviewer context へ渡す。
`security-side-effect-reviewer` と rollback の確認は主に `failure_impact.reasons` を入力にし、
`test-quality-reviewer` と `responsibility-boundary-reviewer` は主に返却 diff 由来の対象リスクを入力にする。
`implementation_complexity` や枝 mode だけを理由に専門 reviewer を選ばない。3つの起動条件が成立しない専門 reviewer を
無条件で起動しない。起動する場合は成立した起動条件、対象リスクまたは確認観点、review 範囲を明示する。
reviewer は最終的な受け入れ判断を行わない。親が diff、テスト、検証結果を確認し、最終的な受け入れを判断する。

専門 reviewer の起動条件はこの節だけが定め、他の節は具体化、起動時に渡す Data の受け渡し規約、
または[枝レビューの4相](branch-review.md) の「再起動対象」のように再起動を定める規約として書く。

### reviewer へ渡すコンテキスト

親は、レビュー対象とリスクに応じて、必要な周辺コンテキストを選択して reviewer へ渡す。
各 reviewer には原則として次の基本情報を渡す。

- タスクの目的と Acceptance Criteria
- 変更対象と commit 範囲
- 変更ファイル一覧と diff text
- reviewer に確認させる具体的な観点

この基本情報は「reviewer 起動テンプレート」の各欄に対応する。diff の受け渡しは同テンプレートに従い、
diff artifact の絶対 path を渡すことを既定とする。

diff だけでは関連する既存設計や利用箇所を判断できない場合は、次を必要に応じて追加する。

- 関連する interface、type、schema
- 主要な呼び出し元
- 関連する既存テスト
- 周辺の directory 構造
- generated file とその生成元
- 変更対象に関係する既存実装
- 外部指示と、`AGENTS.md`、`CLAUDE.md`、`README.md` の関連部分

コンテキストの選択では次を守る。

- repository 全体を無条件に渡さない。
- reviewer の役割に関係しない情報を過剰に渡さない。
- 親の結論だけを渡さず、reviewer が独立して判断できる一次情報を渡す。
- 周辺コードを渡す場合は、なぜ必要なのかを明示する。
- 外部指示と repository 内の指示が競合する場合は、優先関係を明示する。
## reviewer 起動テンプレート

必須完了ゲートの reviewer と専門 reviewer のどちらを起動する場合も、次のテンプレートの全欄を
1項目ずつ埋めて渡す。該当がない欄は「なし」と記入する。欄を空欄のまま残すことと、欄自体を
削除することを禁じる。

```text
- 対象 reviewer: <reviewer 名>
- 確認させる観点: <reviewer に確認させる具体的な観点>
- Branch Plan の failure_impact.reasons: <事前 Data。なければ「なし」>
- 親 QA が返却 diff から特定した対象リスク: <diff 由来 Data。なければ「なし」>
- review 範囲: <対象ファイル・commit 範囲>
- タスクの目的: <実装枝の目的>
- Acceptance Criteria: <AC>
- 親が明示した制約: <なければ「なし」>
- 基準 commit: <SHA>
- 対象 commit 範囲: <SHA range>
- commit log: <対象 commit 範囲の commit log>
- 変更ファイル一覧: <変更ファイル一覧>
- diff artifact の絶対 path: <path。渡さない場合は「なし」>
- diff text（artifact を生成できない場合）: <本文。artifact を渡す場合は「なし」>
- `git status` の結果: <`git status --short` の結果>
- テスト結果: <検証 command の結果>
- 親が選択した周辺コンテキスト: <選択した context。なければ「なし」>
- そのコンテキストを渡す理由: <理由。なければ「なし」>
- 返却してほしい判定: <reviewer に返してほしい判定区分>
- 前回の指摘と親の採否（再起動時）: <前回指摘IDと採否。初回起動なら「なし」>
```

artifact の作成手順は「diff artifact の作成」節に、受け渡し・確認・停止条件は「diff artifact の
受け渡しと停止条件」節に従う。起動前後の記録は「reviewer 起動前後の worktree・親 checkout 照合」節に従う。

「専門 reviewer」節の2つのリスク入力と review 範囲、[返却と統合](qa-and-integration.md) 手順5 の task・AC・commit 範囲・
変更ファイル・diff text を含め、reviewer 起動時に渡す Data はすべてこのテンプレートの
欄として吸収する。テンプレート外に残る起動時 Data はない。
## reviewer 起動前後の worktree・親 checkout 照合

reviewer を起動する直前に、対象 worktree の `git rev-parse HEAD` と `git status --short`、および
親の統合 checkout の `git rev-parse HEAD` と `git status --short` を親が記録する。親の統合 checkout の
記録は diff artifact の書き出し後に取り直し、「返却と統合」手順2 で取得した値を使い回さない。run 開始時に
存在しなかった `.tugite/` は手順2 時点では現れず、artifact 書き出し後に現れるため、手順2 の値をそのまま
使うと reviewer が何もしなくても必ず不一致になる。
reviewer の返却後に同じ4つを取り直し、起動前の記録と一致することを確認する。

`reviewer` 起動テンプレートの `diff artifact` 欄へ `diff artifact` の絶対 path を渡して起動した場合、親は起動直前と
返却後にその artifact の内容 `hash` を記録し、一致することを確認する。`diff text` 欄へ本文を直接記入して起動した
場合は、この確認の対象外とする。

この照合が観測する対象は、対象 worktree の `git rev-parse HEAD` の値、親の統合 checkout の `git rev-parse HEAD` の値、
`git status --short` が示す追跡ファイルの状態（内容変更を含む）と非追跡の項目の増減、および渡した `diff artifact` の内容だけで
ある。非追跡 directory 配下の個別ファイルの増減と内容変更は観測しない。ignore 対象のファイルへの書き込みも観測しない。
観測できない範囲を担保としてどう扱うかは [Reviewer findings の共通契約](reviewer-findings.md) の「read-only の担保」を
正本とする。

一致しない場合、同一 `snapshot` へ一斉起動した全 `reviewer` の `findings` を採用せず破棄する。破棄した `findings` は、
`settled` の定義1番目が言う「その時点までに受け取った全指摘」にも、枝の受け入れ点が言う「未解決または判断未記録の指摘」にも
含めない。破棄した事実と件数を差異の内容とあわせて最終報告へ記録する。
`findings` は親が渡した時点の `snapshot` に対する判定として成立しており、判定の間に対象が動いていれば、
返ってきた指摘が何に対するものかを親が確定できないためである。reviewer が到達するのは対象 worktree
だけでなく親の統合 checkout でもあるため、照合はどちらか一方に絞らず両方に掛ける。

処分は、差異が対象 worktree と親の統合 checkout のどちらに生じたかで決める。両方に差異がある場合は、親の統合 checkout に
差異がある側の処分を採る。

- **対象 worktree の新規非追跡項目だけが差異である場合** — 対象 worktree はこの `run` 専用に作成した一時領域である。
  親は、同一 `snapshot` で起動した全 `reviewer` の返却または停止を確認してから、起動前後の `git status --short` 差分から削除候補を再計算する。
  候補は起動前になかった非追跡の項目のうち、正確な `run-created top-level untracked item` だけとし、削除対象を再検査する。候補 path は対象 worktree の root 内に限定し、
  top-level ではない path、起動前から存在した path、または root 外へ解決される path は unsafe candidate として削除しない。
  検査は symlink を辿らず、symlink はリンク自身だけを削除し、symlink のリンク先を削除しない。
  削除候補について、起動前になかった非追跡の項目を親が削除して記録済み `snapshot` へ戻す。
  削除後に対象 worktree の `git rev-parse HEAD` と `git status --short`、削除後に親の統合 checkout の `git rev-parse HEAD` と `git status --short` を
  起動前の記録へ再照合する。diff artifact を渡した場合は、削除後に渡した diff artifact の内容 hash も起動前の記録へ再照合する。
  削除に失敗した場合、削除対象が残存する場合、追加差異がある場合、
  または unsafe candidate がある場合は、`Needs revision` として統合せず、再実施せずに終了する。
  全条件が成立した場合だけ同じ相を再実施する。再実施は新たな `round` を消費する。
- **対象 worktree のその他の差異** — `HEAD` または追跡ファイルが記録値と異なる場合、および非追跡の項目が減少・消失した場合、
  親は復旧を試みず、当該枝を `Needs revision` として統合しない。
- **親の統合 checkout の差異** — `HEAD` または `git status --short` が記録値と異なる場合、および渡した `diff artifact` の内容 `hash` が一致しない場合、
  親は親の統合 checkout にある項目を自動で復旧・削除せず、当該枝を `Needs revision` として統合しない。不一致になった `diff artifact` は
  以後の `reviewer` 起動へ再利用せず、`run` 完了時の通常の後始末で扱う。

再実施は「打ち切り条件」に従い、`rounds-exhausted` 到達後は同節が定める例外の範囲でだけ行う。

この照合は起動する reviewer を選ばず、すべての reviewer 起動に掛ける。reviewer が対象を動かしうるかは
その reviewer の定義側の設定に依存するため、親はその設定を前提に置かず、自分で観測できる事実だけで
採否を決める。
## diff artifact の作成

reviewer 起動テンプレートの diff artifact 欄へ渡す本文は、基準 commit からの diff をあらかじめ
file へ書き出しておく。

保存先は repository root 相対の `.tugite/diffs/<slug>-diff.patch` に固定する。slug の base の
候補順と正規化手順、Windows 予約名の扱い、衝突時の suffix 選択、ancestor 検査、削除時の再検査、Git
管理は [永続 QA レポート](qa-report.md) の規約を正本として同じ手順に従う。ancestor 検査の対象
component は `.tugite` と `diffs` に読み替える。Markdown file を前提とする path 制約は継承しない。
保持と削除の規約も継承せず、「diff artifact の削除」節で定義する。artifact の path 制約は次のとおり
本 manuscript で定義する。

- target は `.tugite/diffs/` 直下の単一 file に限る。
- 固定の `.tugite/diffs/` prefix を除く file name component に path separator を許可しない。
- `.` または `..` を許可しない。
- 絶対 path を許可しない。

report と別 directory に分けるのは、永続 QA レポートが明示的な削除まで保持される成果物であるのに対し、
diff artifact は reviewer へ diff を渡すためだけの中間物で run 完了時に削除するためである。保持規約の
異なる file を同じ directory へ混在させない。

衝突時は `-diff` を保持したまま `<slug>-diff-2.patch` の順に最初の空きを選ぶ。

本文は worker worktree で取得した `git -C <worktree> diff <base>...HEAD` の出力を、親が転記・要約
せず1回の書き出しでそのまま保存する。保存先 path は親の統合 checkout の repository root を基準に
解決し、ancestor 検査も同じ root で行う。

同一の diff 状態（同じ実装枝、同じ commit 範囲、修正 commit の追加なし）に対する複数 reviewer の
起動では同じ artifact を渡してよい。diff 状態が変わったとき（修正後の再起動、別の実装枝）は
新しい artifact を生成し、変化後に古い artifact を渡さない。

書き出しには、候補 path が既存の場合に上書きせず失敗する Action（例: noclobber を有効にした
redirect）を使う。候補 path の衝突による失敗は次の suffix を選び直す。衝突以外の書き出し失敗は
「diff artifact の受け渡しと停止条件」の確認手順に従い、reviewer へ渡さず再生成するか diff text
経路へ落ちる。artifact 経路を既定とし、diff text の直接受け渡しは同節の停止条件に該当する場合の
例外とする。

保存先 directory が存在しない場合は、ancestor 検査を行ったうえで作成し、作成後に同じ検査を再実行
してから書き出す。
## diff artifact の受け渡しと停止条件

起動 prompt の diff artifact 欄には絶対 path を書く。永続 QA レポートと会話上の報告へ path を
記録する場合は repository 相対 path だけを使う。verbatim 保存される diff 本文はこの記録規則の
適用対象にしない。

diff artifact の欄に path を記入した場合は、reviewer がその file を Read し全文を diff text として
判定根拠にする旨の指示を添える。artifact を生成せず diff text 欄へ本文を直接記入した場合は、その
text を判定根拠にする旨を添える。2つの欄は排他とし、採らなかった側の欄には「なし」と記入して、
両方を同時に有効な指示として残さない。

本文が token、password、cookie、Authorization、private key、`.env` の値、credential 付き URL、
個人情報のいずれかを含む場合、または作成 Action を保証できない場合は artifact を生成せず、diff
text 欄へ本文を直接記入して渡し、生成しなかった理由を記録する。repository 相対 path、コード中の
文字列リテラル、prompt テンプレートの原稿は、diff が構造上含む要素として停止条件に該当しない。

artifact の path を reviewer へ渡す前に、親は diff 全文を自分の context へ読み込まずに書き出し
結果を確認する。確認は書き出し command の exit status が 0 であることと、artifact が空でないこと
の2点で行う。commit を持つ実装枝に対して artifact が空になった場合は、取得元 worktree または基準
commit の指定誤りとして扱う。満たさない artifact は reviewer へ渡さず、「diff artifact の削除」の
手順で削除したうえで再生成し、再生成できない場合は diff text 経路へ落ちる。
## diff artifact の削除

diff artifact は run 完了時に削除する。削除は次の2つの契機で行う。

- 「diff artifact の受け渡しと停止条件」の確認を満たさない artifact を破棄するとき。
- [後始末](run-closeout.md)で、この run に生成した diff artifact をすべて破棄するとき。

削除の前に [永続 QA レポート](qa-report.md) の削除時の再検査を行い、対象が `.tugite/diffs/` 配下の
通常 file であることを確認する。symlink、directory、非通常 file は削除しない。削除後に
`.tugite/diffs/` が空になった場合は directory も削除する。空でない場合は残す。

差し戻しまたは再検証の可能性がある間は後始末の削除を始めない。削除できない artifact は理由と
repository 相対 path を最終報告に含める。
