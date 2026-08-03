# プラン artifact

`plan-craft` Skill が出力する2つの artifact、プラン文書とレビュー状態の正本を定める。確定した
2 artifact は `branch-design` へ渡せるが、受け渡しは親エージェントの責務であり、この Skill は
次工程を開始しない。

## 目次

- 設計方針
- 2つの artifact
- 保存規約
- file 出力と会話内経路
- 書き出しの時点
- レビュー状態のスキーマ
- blocking violation code
- 状態遷移と権限
- branch-design への引き渡し

## 設計方針

- 実装枝・git branch・Branch Plan の用語の書き分けは正本
  [実装枝の準備と委譲](../../impl-lead/references/implementation-branches.md)の
  「用語」節に従う。`impl-lead` と `branch-design` が共有する語彙を
  この Skill でも複製せずそのまま使う。
- プランは実装枝への分割を持たない。分割は `branch-design` の責務である。
- プラン文書の節構成と各節の責務は [起草手順](plan-drafting.md) の
  「プラン文書の節構成」を正本とする。この文書は節構成を再掲しない。
- 1つの設計判断を複数の場所へ別々の言い回しで写すと、レビューは写しの不一致の同期に
  費やされる。設計判断の正本をプラン文書の「設計」節だけに置くのはこの写しを無くすためである。
  以前この案を棄却したのは、分離した artifact と Data が並存して写しが残るためであった。
  棄却理由が成立するのは、並存する Data がプラン文書の内容を構造 field として複製する場合である。
  レビュー状態は構造 field を持たず、プラン文書の内容を1つも複製しない。持つのはレビュー運用の
  状態だけであり、読者も寿命もプラン文書と異なる。したがって写しが生じず、棄却理由は本設計に
  当たらない。確定時の転記そのものが無くなるため、乖離が入りうる1点も消える。
- AC はプラン文書の「Acceptance Criteria」節が保持し、Branch Plan Set の `acceptance_criteria` へ
  原文のまま引き継ぐ。ID 規約の正本は [起草手順](plan-drafting.md) の「AC の書き方」とし、
  この文書は再掲しない。
- レビューの経過は `review.findings` に全 round・全 reviewer 通算の指摘台帳として持つ。指摘 ID
  （`PF-*`）は round をまたいで振り直さず、`reviewer` field で発行元を区別する。verdict は親が
  確認した確定値だけを記録し、reviewer の自己申告をそのまま書かない。
- `validation.blocking` は安定した code を持つ violation の配列とし、承認可否は blocking violation
  の有無だけで決まる。自己評価 boolean は持たない。
- 承認（`approval`）はプランの確定だけを意味し、枝分割・委譲の開始権限を含まない。
  次工程はユーザーの明示的な要求だけを根拠に親エージェントが開始する。

## 2つの artifact

出力は次の2つとし、両者の外に第3の Data を置かない。

- プラン文書 — 起草手順が定める節構成を持つ散文 Markdown 1文書。設計判断の正本であり、人間が
  読む成果物。
- レビュー状態 — レビュー運用の状態だけを持つ YAML Data。

レビュー状態はプラン文書の内容を写す field を持たない。実装目的・要求の所在・AC・scope・依存・
制約の正本はプラン文書の見出し行・「要求の所在」行・各節にある。

## 保存規約

保存先は repository root 相対の `.tugite/plans/` 直下に固定し、プラン文書を
`.tugite/plans/<slug>.md`、レビュー状態を `.tugite/plans/<slug>-review.yaml` とする。

slug の正規化手順、Windows 予約名の扱い、ancestor 検査、Git 管理と保持は
[永続 QA レポート](../../impl-lead/references/qa-report.md) の規約を正本として同じ手順に従い、
この文書へ複製しない。読み替えは次のとおり。

- 固定 prefix は `.tugite/plans/` と読む。ancestor 検査の対象 component は `.tugite` と `plans`
  に読み替える。
- base の候補順は「機密でない task ID または issue 番号 → git branch」と読み、どちらも空になる
  場合の fallback 名は `implementation-plan` と読む。予約名に付ける prefix は `plan-` と読む。
  正本の候補順が title を含み fallback 名が `delegated-implementation` であるのは QA レポート
  向けであり、プラン文書の見出し行は日本語で書かれるため正規化すると base が空になって候補として
  働かない。
- 新規作成時の衝突規約（suffix 選択、symlink / directory / 非通常 file での停止）は継承する。
  suffix 込みの stem の最大80文字は、`-review` と suffix を保持して base の末尾を切る。
- 上書き禁止（exclusive create）は、プラン文書とレビュー状態それぞれの初回作成にだけ継承する。
  同一プランの2回目以降の更新は同じ file を上書きする。

path 制約は継承せず、次の4つを本 manuscript で定義する。正本の path 制約は「reports 直下の単一
Markdown file」という Markdown 前提と一体で書かれており、Markdown 前提だけを外すと残りの制約の
継承可否が決まらない。

- target は `.tugite/plans/` 直下の単一 file に限る。
- 固定の `.tugite/plans/` prefix を除く file name component に path separator を許可しない。
- `.` または `..` を許可しない。
- 絶対 path を許可しない。

2 file は同じ slug を使う。suffix はプラン文書の作成時に選び、レビュー状態はその slug をそのまま
使う。同一プランの更新は同じ path を上書きし、slug を選び直さない。レビュー状態の初回作成でも
slug を選び直さない。`<slug>-review.yaml` が既にある場合は exclusive create が失敗し、
「file 出力と会話内経路」の省略条件「保存先へ書き込めない」に当たる。正本の衝突規約が次の suffix を
選ぶのは1 file だけを書く場合の規定であり、対を崩してまで書き込みを続けない。

保存内容の最小化も同じ正本に従い、次のとおり読み替える。

- 禁止対象のうち `prompt` と `reviewer の生出力` は、プラン文書が設計判断の根拠として引用する
  配布原稿の文言には掛けない。この repository では配布原稿そのものが変更対象であり、掛けると
  原稿改訂プランがプラン文書を生成できなくなる。掛からないのは引用であって、reviewer の応答全文を
  そのまま貼ることは引き続き行わない。
- untrusted field の正規化手順1（改行・control 文字を空白へ置換して単一行にする）と手順2
  （metacharacter の escape。レビュー状態では YAML の quoting と読む）は、レビュー状態が外部由来の
  文字列を引用する field へ適用する。
- 手順3（HTML / link / image の escape）はレビュー状態へ継承しない。YAML file は Markdown として
  描画されず、描画や遷移が発生しない。
- プラン文書の本文そのものには正規化手順1〜3 を適用しない。プラン文書は親が書く散文の成果物で
  あり untrusted field ではない。適用すると文書が単一行へ潰れる。

## file 出力と会話内経路

file 出力を既定とする。file 出力を省略してよいのは次の2条件のいずれかに該当する場合だけとし、
それ以外では常に file を書く。

- 保存先へ書き込めない。ancestor 検査で停止する、安全な create Action を保証できない、書き込みが
  失敗するのいずれか。
- ユーザーが file を出力しないことを明示的に要求した。

省略の判定は artifact ごとに行う。「ユーザーが file を求めていない」ことを省略の条件にしない。
file を明示的に要求しない起動が通常であり、条件にすると同じ入力に対して file を書く実行と
書かない実行の両方がこの規約へ適合する。

会話内経路（`plan_document: 会話内`）は同一会話内で完結する用途に限り、後日渡す経路を持たない。
レビュー状態が file として残らず、後から会話上に貼り直しても `plan-craft` からの再起草になる。

reviewer への渡し方は経路によって変えない。両経路ともプラン文書の全文を起動時に渡し、file の
path を渡して reviewer に Read させる経路は設けない。設けると artifact が空でないことの確認や
内容 hash の照合といった停止条件を plan 段にも定義することになる。

## 書き出しの時点

- プラン文書は起草直後に作成し、採用指摘を反映するたびに同じ path を上書きする。レビューが判定
  する対象は常に最新の1文書であり、round をまたいで版が増えると reviewer へ渡す全文と disk の
  内容が食い違う。
- レビュー状態は `status` を決める確定の時点で作成し、確定をやり直した場合だけ上書きする。round
  の途中でレビュー状態を書かない。書くとレビュー未収束を表す `status` の値が必要になり、`status`
  の値域・有効な組み合わせ表・`review-incomplete` の適用条件・下流の差し戻し規則のすべてに未収束
  という新しい状態を通すことになる。台帳は run の間、親が保持する。
- `awaiting_review` から `approved`（`method: user`）への遷移は確定より後に親エージェントが行う。
  この遷移もレビュー状態へ反映し、`status` と `approval.method` を書き直す。反映しないと、確認
  モードの既定が `review` である経路では file の `status` が常に `awaiting_review` で残る。

## レビュー状態のスキーマ

```yaml
# ============================================================
# レビュー状態 (plan-craft の出力。<slug>-review.yaml)
# ============================================================

plan_document: <プラン文書の repository 相対 path。会話内経路では「会話内」>

status: blocked | awaiting_review | approved
# blocked:          open_questions または validation.blocking のいずれかが非空
# awaiting_review:  confirmation_mode: review で blocking なし。ユーザー承認待ち
# approved:         承認済み。open_questions と validation.blocking がすべて空であることが前提

confirmation_mode: review | auto
# すべての status で保持する。既定は review。auto はユーザーが明示した場合のみ。
# blocked の解消後にどちらへ遷移するかは、この値から復元する

approval:
  method: null | user | auto    # 未承認の間は null。auto は「プランの承認」だけを
                                # 自動化した記録であり、次工程の開始権限を含まない。
                                # termination: round-limit で resolution: unresolved の指摘が残る
                                # 場合は、confirmation_mode: auto でも自動承認しない

open_questions:                 # blocking のみ。1件でもあれば status: blocked
  - question: <確定が必要な問い>
    affects: []                 # プラン文書の節名または AC id

assumptions:                    # minor のみ。AC 充足・scope・実行可否に影響しない仮定
  - topic: <対象>
    assumption: <置いた仮定>
    rationale: <この仮定が blocking でない理由>

review:
  rounds_limit: 10              # 既定10。ユーザー明示時のみ変更
  rounds_completed: 0
  termination: null | zero-findings | trivial-only | round-limit
  findings:                     # 全 round・全 reviewer 通算の指摘台帳
    - id: PF-1                  # 安定 ID。round をまたいで振り直さない
      round: 1
      reviewer: plan-adversarial-reviewer | over-engineering-reviewer
      verdict: 軽微 | 修正推奨 | 修正必須    # 親が確認した確定値
      summary: <指摘の要約>
      resolution: adopted | rejected | unresolved   # unresolved は round-limit 時のみ
      resolution_note: <採用内容または不採用理由>

validation:
  blocking: []                  # violation の配列。1件でもあれば status: blocked
  # - code: <violation code 表の安定 code>
  #   path: <表の区分ごとの値域に従う>
  #   message: <修正に必要な説明>
```

## blocking violation code

親エージェントは reviewer と自身の申告を信用せず、表A をレビュー状態 Data から再計算し、表B を
プラン文書に対する判定で生成する。

**表A: レビュー状態 Data から再計算する code**

| code | 検査内容 |
| --- | --- |
| `duplicate-id` | finding id の重複 |
| `vocabulary-invalid` | `verdict` / `resolution` / `termination` / `reviewer` が定義済み語彙にない値 |
| `state-invalid` | `status` と他フィールドの矛盾。有効な組み合わせ表から再計算する |
| `review-incomplete` | `termination` が null のまま、または過剰実装審査（`reviewer: over-engineering-reviewer` の round）未実行のまま `awaiting_review` 以降へ遷移している |
| `resolution-missing` | `resolution` が未記録の finding、または `resolution: unresolved` が `termination: round-limit` 以外で残っている |
| `rounds-invalid` | `rounds_completed` が `rounds_limit` を超えている、または `findings[].round` と矛盾する |

表A の `path` はレビュー状態のスキーマ上の path を書く（例: `review.findings[2].resolution`）。

**表B: プラン文書に対する親の判定で生成する code**

| code | 検査内容 |
| --- | --- |
| `body-missing` | プラン文書の本文が無い、または空 |
| `handoff-incomplete` | 見出し行、「要求の所在」行、「Acceptance Criteria」節、「scope」節のいずれかに記載が無い |

表B の `path` はプラン文書の位置を指す語（節名、`見出し行`、`要求の所在行` のいずれか）を書き、
本文そのものが無い場合は `plan_document` と書く。

表Bの「記載が無い」は、行または節が存在しない、中身が空、または「該当なし」だけである場合を指す。
節見出しの有無だけでは判定しない。「Acceptance Criteria」節と「scope」節は `branch-design` の
入力要件そのものであり、内容が無いプランは引き渡せないため、「該当なし」だけの記載で充足したと
みなさない。

`body-missing` が見るのはプラン文書の本文の有無であって file の有無ではない。file 出力経路では
file の本文、会話内経路では会話上に提示した本文を対象にする。file を書かない経路で
`body-missing` を立てると、その経路のプランが常に `blocked` へ固定され、file 出力の省略を認める
規定が実行不能になる。

`body-missing` を立てるのは本文全体が無い、または空の場合だけとし、本文がある場合の個別の欠落は
`handoff-incomplete` だけで扱う。1つの欠落に2つの code を立てない。両 code はプラン文書という
同一対象を見るため、分界を置かないと本文が空のプランで2つの code が同時に成立し、1つの欠落に
2つの修正要求が出る。

本文が規定の節見出しを備えているかは意味判断であり、レビュー状態 Data から再計算できない。表を
2つに分けるのはこのためである。意味判断を表Aへ入れると、表A 全体が Data から再計算できるという
性質が壊れる。表Bの生成主体は親の判定であり、節の充足は起草手順とレビューの判定が担う。Branch
Plan Set 正規スキーマの `branch-contract-violation` が機械検査ではなく判定で生成される先例に従う。

廃止した `scope-conflict` と `unknown-reference`、AC id に対する `duplicate-id` は plan 段では
扱わない。`scope-conflict` は `allowed_paths` の廃止により下流の検査もなく、code 自体を廃止した。
AC id に対する `duplicate-id` だけは `branch-design` が Branch Plan Set 正規スキーマの同名 code で検査する。
`unknown-reference` は、plan 段で id を参照する field が `open_questions[].affects` だけになり、
その値をプラン文書の節名または AC id とすることで参照検査の対象が残らない。finding id に対する
`duplicate-id` は表Aに残る。

トップレベル状態は値を個別に検査せず、次の有効な組み合わせ表から検査する。表にない組み合わせは
`state-invalid` を生成する。

| status | approval.method | confirmation_mode |
| --- | --- | --- |
| `blocked` | `null` | `review` / `auto` |
| `awaiting_review` | `null` | `review` のみ |
| `approved` | `user` | `review` のみ |
| `approved` | `auto` | `auto` のみ |

## 状態遷移と権限

| 遷移 | 実行主体 | 条件 |
| --- | --- | --- |
| (生成) → `blocked` | planning Skill | `open_questions` または `validation.blocking` が非空 |
| (生成) → `awaiting_review` | planning Skill | `confirmation_mode: review` かつ blocking なし |
| (生成) → `approved` (`method: auto`) | planning Skill | `confirmation_mode: auto` かつ blocking なし、かつ `resolution: unresolved` の指摘なし |
| `blocked` → `awaiting_review` | planning Skill（再実行） | 原因解消後に全 validation を再実行して blocking なし、`confirmation_mode: review` |
| `blocked` → `approved` (`method: auto`) | planning Skill（再実行） | 同上、`confirmation_mode: auto`、かつ `resolution: unresolved` の指摘なし |
| `awaiting_review` → `approved` (`method: user`) | 親エージェント | ユーザーの承認操作。blocking violation が残る場合は承認操作があっても遷移しない |

承認と次工程の開始は独立している。`awaiting_review` から承認された場合も、枝分割・委譲の要求が
なければプランの確定だけで停止する。確認モードの既定値は `review` とし、`auto` はユーザーが明示した
場合のみ使う。

## branch-design への引き渡し

確定した2 artifact は、`branch-design` の「入力の確認」が求める項目へ次のとおり対応する。
受け渡しは親エージェントの責務であり、この Skill は `branch-design` を起動しない。
`branch-design` 側の入力要件は変更しない。

| branch-design の入力 | プラン文書の位置 |
| --- | --- |
| 実装目的 | 見出し行 |
| 元プラン | 「要求の所在」行 |
| Acceptance Criteria（原文） | 「Acceptance Criteria」節（ID ごと原文のまま） |
| 変更禁止範囲 | 「scope」節の変更禁止 |
| 既知の依存 | 「依存 / 制約 / 前提 / 未確定」節の依存 |

`handoff-incomplete` は、この表の左列を充足できない欠落を検査する。「既知の依存」が対象に入らない
のは、依存が無いプランが正当に存在し、「該当なし」が正しい記載になるためである。

左列は `branch-design` の入力要件そのものであり、行を足すことは入力要件の変更になる。
