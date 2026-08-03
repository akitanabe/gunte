---
name: feature-lead
description: >-
  ユーザー要求から実装完了までを、`plan-craft` / `branch-design` / `impl-lead` の3段を
  順に連結して一括で進める orchestration skill。プランから実装までの一括実行を明示的に
  要求されたときに使う。確定済みのプラン文書とレビュー状態を渡して実装までの一括実行を
  要求されたときも発火し、`branch-design` から開始する。各段の判断基準は再定義せず、
  段の遷移と判断点の処理だけを担う。既定では段が `blocked` を返した時点で停止し、
  `unattended` の明示時だけ自律解決して進む。
---
<!-- Generated from shared/. Do not edit directly. -->

# 要求から実装までの一括進行

ユーザー要求を `plan-craft` → `branch-design` → `impl-lead` の順に通し、実装完了までを
一括で進める。この Skill が担うのは段の連結と判断点の処理だけであり、プランの起草、枝分割、
実装と QA の中身はすべて各段の Skill が担う。

## この Skill の責務

- 出力は最終的に `impl-lead` の最終報告である。この Skill 自身はプラン、枝、diff を作らない。
- 各段の判断基準を再定義しない。AC の書き方、枝分割の基準、枝 mode の導出、QA の受け入れ判断は
  すべて各段の Skill の正本に従う。この Skill に判断表を複製しない。
- 判断点の扱いは `autonomy` に従う。既定の `attended` では、段が `blocked` を返した判断点を
  ユーザーへ返して確定を求める。`unattended` ではこの Skill が自律解決して進める。どちらの場合も
  判断点を黙って消さず、判断点台帳へ記録する。
- 開始段より前の段は実行せず、開始段以降の段は飛ばさない。`branch-design` を省いて
  プラン文書とレビュー状態を直接 `impl-lead` へ渡さない。

## 発火条件

- ユーザーがプラン作成から実装までの一括実行を明示的に要求したとき。
- ユーザーが確定済みのプラン文書とレビュー状態を渡して実装までの一括実行を要求したとき。この場合は
  `branch-design` から開始する。

開始段は入力の起点で一意に決める。レビュー状態が `status: approved` であり、その `plan_document`
が repository 相対 path であり、その path のプラン文書が読める場合だけ `branch-design` から
開始する。それ以外の入力（自然文の要求、`status` を持たないレビュー状態、プラン文書だけを渡された
入力、issue 本文や会話内のプラン）はすべて `plan-craft` から開始する。プラン文書だけを渡された
場合はレビュー状態が無いものとして扱い、その path から兄弟のレビュー状態を解決しない。レビュー状態は
承認の記録であり、推測で解決すると、そのプラン文書に対応しないレビュー状態を承認済みとして扱う経路が
開くためである。命名規約が2 file の対を保証するのは同一 run が両方を書いた場合だけであり、片方だけが
残る状況を命名からは区別できない。`plan_document: 会話内` のレビュー状態は、プラン文書を会話上に貼り直されても開始段判定を通さず、
`plan-craft` から開始する。会話内経路は同一会話内で完結する用途に限られ、後日渡す経路を持たない
ためである。ただし `status` が `awaiting_review` または `blocked` のレビュー状態はこの既定規則の
対象から除き、次の差し戻し規則を優先する。

次の場合は発火しない。

- `status` が `awaiting_review` または `blocked` のレビュー状態を渡されたとき。
  承認または判断点の確定を求めて差し戻す。これを判断点として台帳へ記録しない。判断点は段が
  返したものだけを対象とし、起動前に渡された入力を「判断点の分類」にも再実行にも掛けない。
- 確定済みの Branch Plan Set を渡されたとき。この Skill の対象外であり、`impl-lead` を直接使う経路を
  案内する。
- 単一の段だけの要求。プランのみは `plan-craft`、枝分割計画のみは `branch-design` の責務である。
- `direct` の明示時。委譲を伴わない経路であり、この Skill の対象外である。
- テストスイートの棚卸し要求（`test-audit` の責務）。

## 入力の確認

着手前に次を確認する。不足が blocking なら補完せず、開始段に応じた受け手へ渡す。`plan-craft` から
開始する場合は `plan-craft` の `open_questions` として扱わせ、`branch-design` から開始する場合は
`branch-design` の `unresolved_decisions` として扱わせる。

- 対象 repository と読み取り可能な現状。
- 既知の制約・依存。
- `autonomy` の明示指定。既定は `attended` とし、ユーザーが全面委任を明示した場合だけ
  `unattended` を選ぶ。タスク規模、段の停止回数、進行の停滞を理由にこの Skill 側で
  `unattended` へ切り替えない。
- mode 指定（`lite` / `standard(-adaptive)` / `strict(-adaptive)` / `strict-full`）。
  未指定の場合はこの Skill で補わず、`impl-lead` の既定に委ねる。

次は `plan-craft` から開始する場合の確認項目である。`branch-design` から開始する場合は、確定済みの
プラン文書とレビュー状態がこの位置を占め、受け手のない確認項目を残さない。

- 要求原文。言い換えず、`plan-craft` へそのまま渡す。
- `rounds_limit` の値の明示指定。`plan-craft` へそのまま渡す。

`strict-full`（`policy: fixed` かつ `baseline: strict`）が指定された場合は、`autonomy` の
`unattended` を `attended` へ強制的に落とす。判定と確定は「入力の確認」の時点で行い、`plan-craft`
を起動する前に `autonomy` を確定して以降の全段へ適用する。`impl-lead` 段へ入ってから落とす実装を
許さず、計画段の判断点も自律解決しない。`strict-full` は枝数を提示したユーザー確認を委譲開始の
条件とする語彙であり、判断点の自律解決と論理的に両立しない。落とした事実と理由を最終報告へ記録し、
ユーザーが明示した `unattended` を黙って無効化しない。

## 全体の流れ

1. 上の入力を確認し、開始段を決める。
2. `plan-craft` から開始する場合は、`plan-craft` を `confirmation_mode: auto` で起動し、
   プラン文書とレビュー状態を得る。`branch-design` から開始する場合はこの段を実行せず、渡された
   確定済みのプラン文書とレビュー状態をそのまま次段の入力にする。
3. `plan-craft` を実行した場合は「段の遷移と判断点の処理」に従い status を判定する。判断点が
   あれば `autonomy` に従って処理する。
4. 確定したプラン文書とレビュー状態を `branch-design` へ `confirmation_mode: auto` で渡し、
   Branch Plan Set を得る。
5. 「段の遷移と判断点の処理」に従い status を判定する。判断点があれば `autonomy` に従って
   処理する。
6. 「授権の根拠」に従い、対象 Branch Plan の `delegation` を設定する。
7. Branch Plan Set と判断点台帳を `impl-lead` へ渡す。`impl-lead` は受け入れ口の再検証を通常
   どおり行い、`order` に従って Branch Plan を実行する。
8. `impl-lead` が未授権の Branch Plan の境界で止まった場合は、手順9を行わずに「Branch Plan
   境界の停止」に従って処理する。ユーザーが授権を確定したら、その Branch Plan について手順6から
   行い直す。
9. `order` の全 Branch Plan の実行が終わったら、`impl-lead` の最終報告を提示する。段ごとの
   要約を先に置き、最終報告を末尾に置く。判断点台帳の全件を会話上の最終報告にも含める。

## 段の遷移と判断点の処理

各段は `confirmation_mode: auto` で起動する。一括実行の明示要求そのものが `confirmation_mode: auto`
の明示指定と委譲要求を兼ねる。ユーザーが `confirmation_mode` または委譲の要否を明示した場合は
その明示を優先し、この読み替えは明示がない場合にだけ適用する。`auto` は各段の承認だけを自動化する
もので、判断点の自動解決ではない。判断点の扱いを決めるのは `autonomy` であり、`confirmation_mode`
と混同しない。

次のいずれかを判断点として検出する。

- `plan-craft` が `status: blocked` を返した（`open_questions` または `validation.blocking` が
  非空）。
- `plan-craft` が `termination: round-limit` で `resolution: unresolved` の指摘を残した。
- `branch-design` が返した Branch Plan Set のうち1件でも Branch Plan が `blocked` である
  （`blocked` の定義は `branch-plan-schema.md` に従う）。この場合は特定の Branch Plan ではなく
  段全体を判断点として扱う。
- `impl-lead` が各 mode のゲートで停止した。停止条件は `impl-lead` の契約に従う。ただし
  Branch Plan 境界（未授権の Branch Plan への到達）での停止はこれに含めない。「Branch Plan
  境界の停止」に従う。

### 判断点の分類

判断点は `origin` によって resolvable と non-resolvable に分類する。

resolvable は `origin: open_questions` / `origin: unresolved_decisions` /
`origin: validation.blocking` と、ユーザーが `rounds_limit` の値を指定していない場合の
`origin: round-limit` である。再実行の入力には、`open_questions` と `unresolved_decisions` では
確定した解決内容を、`validation.blocking` では violation の `code` / `path` / `message` そのものを
載せ、`round-limit` では「`round-limit` の扱い」に従って引き上げた `rounds_limit` を載せる。
これらはいずれも同じ条件で再実行を発火する。

non-resolvable は `origin: impl-lead-gate` と、ユーザーが `rounds_limit` の値を指定した場合の
`origin: round-limit` である。`origin: impl-lead-gate` は一律 non-resolvable とする。`impl-lead`
側の停止に planning Skill の再実行で対応する遷移が存在しないためである。値を指定した場合の
`origin: round-limit` を non-resolvable とするのは、ユーザーが指定した上限の値を黙って超えない
ためである。いずれも再実行の対象にせず停止する。

`mode-proposal-invalid` は「`delegation.requested_mode` の設定」の写像規約により `delegation` の
設定時点で回避されるため、判断点として発生しない。

分類が一意に決まらない判断点は non-resolvable として扱い停止する。状態遷移の主体は既存の
状態遷移表にある planning Skill の再実行だけを使う。

### 再実行の枠

再実行の計数は段側で数え、各段の再実行は一括実行を通じて1回までとする。再実行で新しい判断点が
生じた場合もその段の枠は消費済みとして停止する。この枠は `unattended` の自律解決による再実行に
だけ掛け、`attended` でユーザーが判断点を確定した後の再実行は計数しない。

同一段が resolvable と non-resolvable を同時に返した場合は、non-resolvable が1件でもあれば
再実行せず停止する。同一段が `origin: round-limit` と `origin: validation.blocking` を同時に
返した場合、ユーザーが `rounds_limit` の値を指定していなければ `round-limit` を優先し、
「`round-limit` の扱い」の引き上げ再実行を1回として行う。値を指定している場合は `round-limit` が
non-resolvable であるため優先規則を適用せず停止する。

再実行後も残る判断点と non-resolvable な判断点は、`unattended` でも停止してユーザーへ返す。

### `attended`（既定）

判断点を検出した段で停止する。どの段のどの判断点で止まったかを明示してユーザーへ返す。ユーザーが
判断点を確定した後は停止した段から再開し、それより前の段の出力は破棄しない。

### `unattended`

停止せず、「自律解決の規律」に従って判断点を解決してから次段へ進む。解決は判断点の記録を伴い、
記録できない解決は行わない。

### Branch Plan 境界の停止

`impl-lead` が未授権の Branch Plan の境界で止まることは、上の判断点のいずれにも当たらない。境界の
停止を判断点として扱わない理由は「判断点台帳」に従う。授権する Branch Plan の範囲は「授権の根拠」
に従う。

- `attended`（既定）では、`impl-lead` が境界で止まったら、`impl-lead` が提示した内容をそのまま
  ユーザーへ返し、次の Branch Plan の授権を求める。この提示は手順9の最終報告とは別の、会話上の
  中間提示である。提示内容そのものは `branch-plan-intake.md` を正本とし、この Skill へ複製
  しない。ユーザーが授権を確定したら、「全体の流れ」の手順6からその Branch Plan について行い直し、
  `impl-lead` を再開する。この停止は Skill の責務を果たさずに終了することではなく、Branch Plan
  を承認単位にした結果として意図された停止である。
- `unattended` では境界で止まらない。境界を通過した事実と通過した Branch Plan id を最終報告へ
  記録する。

## `round-limit` の扱い

`unattended` で `origin: round-limit` の判断点を検出した場合の扱いは、ユーザーが `rounds_limit` の
値そのものを入力で指定したかどうかで分ける。値の指定がない場合に限り、`rounds_limit` を既定値と
同じ幅（10）だけ引き上げて段を再実行する。値の指定がある場合は引き上げず停止する。ユーザーが
上限の値を指定した意図（コストや時間の制約）を、起動要求という弱い暗黙の授権で上書きしない
ためである。

引き上げは「再実行の枠」に含めて段ごとに1回までとし、再実行後も `round-limit` に達する場合は
停止する。`plan-craft` の `rounds_limit` の引き上げをユーザーの明示に限る契約は変更せず、値の
指定がない場合に限りこの Skill の起動要求が引き上げの明示を兼ねるという読み替えで授権する。値の
指定と起動要求による授権を同じ「明示」として扱わない。引き上げた事実と引き上げ後の値を判断点台帳
と最終報告へ記録する。

## 自律解決の規律

`unattended` の自律解決は次に従う。ユーザーへ判断を返さないことと、判断の根拠を持たないことは
別である。根拠源と scope の基準は開始段に応じて一意に定める。`plan-craft` から開始する場合は
要求原文をこれに充て、`branch-design` から開始する場合は確定済みプラン文書の見出し行と「scope」節と
「Acceptance Criteria」節が要求原文の位置を占める。

- 根拠は根拠源と repository の観測可能な事実（既存 code、テスト、規約、設定）から取る。
  根拠を取れる場合は `basis_kind: observed` として記録する。
- 根拠を取れない判断点も停止せずに解決するが、`basis_kind: assumed` として仮定であることを
  記録する。仮定を観測事実として記録しない。
- 根拠源の scope を広げる方向で解決しない。判断点を解くために機能、対象範囲、成果物を足さない。
  scope に関する判断点は、根拠源が支持する最小の解釈を採る。
- 段の判断基準そのものを緩めて解決しない。AC を落とす、必須テストを省く、枝 mode を下げる、
  blocking violation を無視するといった手段で判断点を消さない。判断基準を満たす解を選ぶ。
- 複数の解が規律を満たす場合は、後から戻しやすい方を選ぶ。

不可逆な操作（外部への公開・送信、tracked file の削除、履歴の書き換え、外部状態の変更）は
`unattended` でも自律解決の手段に含めない。当該操作を伴わずに到達できる範囲まで進め、当該操作を
要した既存の判断点へ `resolved_by: deferred` を付与し、操作が必要だった事実をその判断点の
`resolution` と `basis` へ記録して、未実施のままユーザーへ返す。実行の
可否は判断点ではなく実行時の承認事項であり、`unattended` の授権はこれを含まない。

<!--
Why Not: `basis_kind: assumed` を認めることが「blocking な不足を仮定で補完しない」規定と
両立する理由。後者は起動前の入力に掛かる規定（「入力の確認」）であり、前者は段が判断点として
返した後の記録区分（「自律解決の規律」）である。適用範囲が重ならないため両立する。両者を同じ
「仮定の禁止」で扱うと、根拠を取れない判断点が必ず停止になり `unattended` が成立しない。
-->

## 判断点台帳

判断点は検出時に台帳へ記録し、会話上の最終報告へ全件を記載する。台帳は `impl-lead` へも渡す。
[永続 QA レポート](../impl-lead/references/qa-report.md) を生成する場合は同じ台帳をその入力に
する。永続 QA レポートの記録項目の規定は本 skill の範囲外であり、正本は `impl-lead` 側に置く。

| field | 値 |
| --- | --- |
| `stage` | `plan-craft` / `branch-design` / `impl-lead` |
| `point` | 判断点。原文を言い換えずに保持する |
| `origin` | `open_questions` / `unresolved_decisions` / `validation.blocking` / `round-limit` / `impl-lead-gate` |
| `resolution` | 確定した内容、または未確定として記録した事実 |
| `basis` | 根拠 |
| `basis_kind` | `observed` / `assumed` |
| `resolved_by` | `user`（`attended` での確定） / `autonomous`（`unattended` での自律解決） / `deferred`（解決も確定もされないままユーザーへ返した） |

`resolved_by: deferred` は、解決も確定もされないままユーザーへ返したすべての判断点に用いる。
不可逆操作を要した判断点、non-resolvable な判断点、同一段の停止に巻き込まれて未着手のまま返した
resolvable な判断点がこれに当たる。`deferred` は解決を試みていた既存の判断点へ付与し、新しい
台帳行を起こさない。

台帳の記載は `attended` / `unattended` の双方で必須とする。判断点をユーザーへ返さない解決が
含まれる場合、この記載が唯一の検分経路になる。記載できない判断点は解決したとみなさない。

Branch Plan 境界の停止（`impl-lead` が未授権の Branch Plan に到達して止まること）は判断点では
ないため、`origin: impl-lead-gate` として判断点台帳へ記録しない。台帳が対象とするのは段が返した
判断点であり、境界の停止は授権が未設定であることの帰結だからである。

`basis_kind: assumed` の項目は台帳内で区別して示し、観測事実に基づく解決と混ぜない。仮定の総数を
最終報告の冒頭要約にも出す。

## 授権の根拠

`branch-design` は Branch Plan Set の各 Branch Plan について `delegation.authorized: false` を
返す。この Skill は親 Codex エージェントの役割で、Branch Plan ごとに独立して
`delegation.authorized: true` / `authorized_by: user` を設定する。根拠はこの Skill の起動要求
そのものであり、要求は plan から実装委譲までを含むものとして扱う。

授権を設定するのは、Set の全 Branch Plan が `status: approved` を返した場合だけである。1件でも
`blocked` な Branch Plan があれば段全体を判断点として扱い停止し、一部の Branch Plan だけを
授権して進める経路は持たない。

授権する Branch Plan の範囲の正本は `branch-plan-intake.md`「Branch Plan 境界の授権」である。
この Skill は `autonomy` への具体化だけを行う。

- `attended`（既定）では、`order` の先頭の未実行 Branch Plan だけを授権する。正本が既定として
  述べる範囲と同じである。
- `unattended` では、Set の全 Branch Plan を授権する。`autonomy: unattended` の明示を、正本が
  例外とする「ユーザーが全 Branch Plan の一括授権を明示した場合」として読み替える。
  `round-limit` の扱いで起動要求が引き上げの明示を兼ねると読み替えるのと同じ形である。

段ごとの委譲要求の再取得は求めない。ただし `impl-lead` の受け入れ口が行う再検証は省略しない。

### `delegation.requested_mode` の設定

ユーザーが明示した mode の写像は、`impl-lead` SKILL.md の入力語彙の写像表を正本として参照する。
mode が未指定の場合は `requested_mode` を `null` のまま保持する。

`delegation_mode_proposal` は planning Skill の生成物であり、親 Codex エージェントが書ける範囲は
`delegation` 配下だけである。この Skill は `delegation_mode_proposal` を書かない。写像した
`requested_mode` が枝の `failure_impact.level` と整合せず、`branch-design` の branch-plan-schema.md の
出力条件表が proposal を要求する組み合わせ（`policy: fixed` かつ `baseline: lite` で `medium`
以上の枝を含む）になる場合は、`delegation` を設定するその時点で、表が提案する
`{policy, baseline}` を `requested_mode` へ設定する。設定後は `policy: adaptive` になり表は
proposal を要求しないため、proposal を書かずに `mode-proposal-invalid` を回避できる。
proposal の判定には枝の `failure_impact.level` を使い、`implementation_complexity` は使わない。

この設定は `delegation.authorized` の `false` から `true` への遷移1回の中で完結し、状態遷移表の
親の権限行に収まる。判断点を発生させないため台帳へ新しい行を起こさない。引き上げ先は表が決める
ため、この原稿へ表を複製せず正本を参照する。引き上げた事実と引き上げ前後の `{policy, baseline}`
は最終報告へ記録する。ユーザーが明示した mode と異なる値を設定するため、この引き上げの根拠は
`impl-lead` SKILL.md の引き下げ禁止の例外条項に置く。この引き上げは `autonomy` に依らず
`attended` と `unattended` の双方で適用する。`attended` でも proposal を生成できる主体が存在せず、
ユーザーが確定しても解けない停止になるためである。

<!--
Why Not: この Skill が各段の判断表（枝 mode の決定表、blocking violation code、QA 基準）を
持たない理由。判断表を複製すると正本が二重化し、片方だけ改訂されたときに一括経路と単段経路で
挙動が食い違う。この Skill は遷移だけを持ち、判断は各段の正本へ委ねる。

Why Not: `unattended` を `confirmation_mode: auto` へ統合しない理由。`auto` は各段が自分の
出力を承認する範囲の自動化であり、判断点が残る場合は `auto` でも `blocked` を返す契約になって
いる。両者を同じ語彙にすると、段の承認自動化を求めただけの利用者が判断点の自律解決まで
受け取ることになる。
-->
