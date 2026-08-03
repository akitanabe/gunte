<!-- Generated from shared/. Do not edit directly. -->

# Reviewer findings の共通契約

## 目次

- 位置づけ
- read-only の担保
- 指摘件数のサマリ行
- 指摘ごとの evidence

## 位置づけ

findings を返すすべての reviewer が共通で満たす2点を定義する。正本はこの reference が持ち、
各 reviewer 原稿は同じ2点を自分の出力形式の語彙で本文に書き下す。

対象は `responsibility-boundary-reviewer`、`test-quality-reviewer`、`security-side-effect-reviewer`、
`writing-principles-reviewer`、`over-engineering-reviewer`、`plan-adversarial-reviewer` とする。
指摘 Data を返さない `expert-selection-reviewer` と `review-patch-refactorer` は対象外とする。

判定語彙、0件の表記、判定対象外の範囲の書き方は各 reviewer 原稿を正本とし、この reference では変更しない。

ここで定めた対象は上記2点だけに適用する。「read-only の担保」は対象範囲が異なり、同節が自身の対象を定める。

## read-only の担保

指摘 Data を返すだけの reviewer には、ファイルを書き換える tool を渡さない。担保の実体は Claude 向け
agent frontmatter の `tools`（許可 tool の allowlist）であり、`Edit` / `Write` / `NotebookEdit` を
含めないことでこれらの tool が渡らない。`disallowed_tools` にも同じ3つを重ねて書くが、これは意図を
明示する重ね書きであり、`tools` を伴わずに単独で担保になるものではない。Codex 向けは `sandbox_mode` の
`read-only` が同じ役割を果たす。原稿の指示文だけに委ねると、reviewer が指摘を返す代わりに対象を直して
しまう余地が残るためである。

探索手段は責務で分ける。判定に検証の実行や基準 commit 時点のファイル参照が必要な reviewer には
`Bash` を渡し、渡された Data のテキストだけで判定できる reviewer には渡さない。実行できない reviewer が
「実行すれば分かること」を推測で書く状態と、テキストで足りる reviewer に実行手段が余る状態の、
どちらも避けるための分け方である。

どの reviewer がどちらに属するかはこの節に列挙せず、各 agent 定義を正本とする。ここに一覧を置くと、
reviewer を増やすたび原稿と定義の2箇所を揃えることになり、片方だけが古い状態を作るためである。

`Bash` を渡した reviewer について、Claude 側で書き込みを禁じているのは原稿の指示文だけである。
`Bash` からファイルを書けるため、tool 単位の制限（`tools` の allowlist からの除外や `disallowed_tools`）
はいずれも迂回でき、Codex 側の `sandbox_mode` のように機構としては禁じられない。担保の強さは platform
間で非対称であり、これは `Bash` を渡す判断に伴う既知の制約として引き受ける。

`Bash` を渡した reviewer は、対象 worktree に限らず、自身が到達できるいかなる repository
（対象 worktree、親の統合 checkout など）に対しても、読み取りと検証の実行だけを行い、追跡ファイルを
変更しない。書き込みは、対象とした repository の外の一時領域へ作成した複製に限り、それ以外の
いかなる path へも書き込まない。書き込みを repository 単位でしか条件付けないと、`~/.bashrc` や
`~/.ssh/config`、plugin の install 先のようにどの repository にも属さない書き込み先が射程外になり、
親が突き合わせる git 状態にも現れないため、run 限りのはずの権限が run を越えて永続化しうる。
ミューテーション注入や検証用の複製のように書き込みを伴う検証は、`mktemp -d` などで新規作成した
一時 directory 配下へ複製して行う。固定名の directory を再利用すると、既存内容ごと再利用・削除する
余地が残るためである。複製は run 中に削除し、削除の対象は自分が作成したその複製 directory に限る。
削除できない場合は path を返却物へ記録する。worktree を丸ごと複製すると非追跡の `.env` や credential も
複製されかねないため、複製対象に非追跡ファイルを含めない。あわせて、HEAD・refs・object DB・git 設定・
hooks を変更する操作、および到達可能性
や reflog を失わせる操作を行わない。例えば `commit` / `checkout` / `switch` / `reset` / `stash` /
`rebase` / `merge` / `cherry-pick` / `worktree add` / `worktree remove` / `branch -d` /
`branch -D` / `branch -f` / `branch -m` / `update-ref` / `symbolic-ref` / `reflog expire` /
`gc --prune=now` / `config` の変更 / `.git/hooks/*` への書き込み / `clean -fdx` / `restore` /
`push` が該当する。追跡ファイルの編集は親の照合で気づけるが、これらは状態を戻せば照合をすり抜けうるためである。

この作業範囲も上記と同じ理由で tool metadata では強制できない。したがってここで定めるのは契約であり、
担保は各 reviewer 原稿の指示文と、親が起動前後に行う照合になる。検査の対象と手順は
[Reviewer の起動と diff の受け渡し](reviewer-dispatch.md) の
「reviewer 起動前後の worktree・親 checkout 照合」に従う。
この照合は `impl-lead` の委譲経路が対象 worktree を持つことを前提にした手順であり、`plan-craft` の
プラン審査のように対象 worktree を持たない起動経路では、担保は各 reviewer 原稿の指示文だけになる。
network 送信（`curl` / `gh api` / `ssh` などによる外部送信）と credential の参照（`~/.git-credentials`
や `.env` の読み取りなど）は、この契約の担保対象外である。`push` の禁止にも上記の検査にも現れないため、
機構的に扱われていると誤解しないこと。

この節の対象は、上記2点の6本に `expert-selection-reviewer` を加えた reviewer 7本とする。
`expert-selection-reviewer` は指摘 Data を返さないため上記2点の対象外だが、ファイルを変更しない点は
共通であり、この節では対象に含める。指摘された範囲を修正する `review-patch-refactorer` は書き込みを要するため、
この節でも対象外とする。

## 指摘件数のサマリ行

応答の冒頭に、指摘件数を1行で読み取れるサマリ行を置く。親が返答本文を数え直さずに指摘の有無と
規模を確定できるようにするための契約であり、件数の書式は規定しない。

置き方は出力形式の先頭に判定項目があるかで2通りに分かれる。

- 出力形式の先頭に判定項目を持つ reviewer は、その判定項目と同じ行に件数を示す。別のサマリ行を追加しない。
- 判定項目を持たない reviewer は、件数だけを示すサマリ行を冒頭に置く。判定項目を新設しない。

指摘0件でもサマリ行を省略しない。0件であることを表す語は、各 reviewer が既に使っている語をそのまま使う。

## 指摘ごとの evidence

指摘ごとに、その指摘が実在することを親が確かめられる evidence を示す。次の3つのうち、いずれか1つを示せばよい。

- 該当ファイルと行の引用
- 再現手順
- 参照した Data の path と id

3択にしてあるのは、diff を持たない工程の reviewer でも evidence を示せるようにするためである。
プランや Data だけを入力に取る reviewer は、参照した Data の path と id で足りる。

evidence は各 reviewer が既に持つ場所または根拠の項目の中で示す。evidence 専用の項目を新設しない。
