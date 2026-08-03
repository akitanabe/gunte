<!-- Generated from shared/. Do not edit directly. -->

# 報告形式

Test Inventory Data をユーザーへ提示する形式の正本を定義する。

## 目次

- 提示の順序
- 標準テンプレート
- 確認操作
- 記入規則

## 提示の順序

`status` に応じて提示内容を変える。

- `complete`: 概況 → テスト一覧表 → gap 指摘一覧 → Test Inventory Data の YAML 全文 → 確認操作の
  順で提示する。
- `partial`: 概況の直後に走査できなかった範囲を提示してから、テスト一覧表以降を続け、最後に確認操作を
  提示する。読めなかった範囲を先に示し、一覧表・gap 指摘・確認操作の対象が走査対象全体を網羅した
  結果ではないことを明示する。
- `blocked`: `validation.blocking` を先に提示し、解消を依頼する。この状態ではテスト一覧表と
  gap 指摘一覧を出さず、確認操作も提示しない。

## 標準テンプレート

次のテンプレートを使用する。field には Test Inventory Data の値だけを記入する。

```markdown
# テスト棚卸し報告

## 概況

- スコープ: <scope.requested>
- status: `complete` / `partial` / `blocked`
- 走査規模: <ファイル数> file / <テスト数> test
- gap 指摘: <findings 件数>(subject 単位 <n> / entry 単位 <n>)

## 走査できなかった範囲(partial のときのみ)

| path | 理由 |
| --- | --- |

## テスト一覧

| ID | テスト名 | 対象 | 分類 | 検証している振る舞い |
| --- | --- | --- | --- | --- |

## gap 指摘

| ID | code | 対象 | 概要 | 推奨対応 |
| --- | --- | --- | --- | --- |

指摘がない場合は `該当なし`。

## Test Inventory Data

<YAML 全文>
```

## 確認操作

報告を受けたユーザーへ次の操作を提示する。

- 報告のみで終了 — 既定。この操作を選んだ場合、この Skill は何も起こさない。
- 指摘の解消を計画 — 対象の gap 指摘 `G-*` をユーザーが指定し、`branch-design` へ
  渡して実装枝計画へ進める。

「指摘の解消を計画」は、gap 指摘が `該当なし` の場合は提示しない。指定できる解消対象がないため
である。

`branch-design` へ渡すのは親エージェントの責務であり、この Skill は
`branch-design` を直接起動しない。この Skill が担うのは報告と確認操作の提示までで
あり、実装枝計画の生成や委譲の開始は行わない。

## 記入規則

- テスト一覧表の行は `inventory` の記載順と一致させる。走査順を変えずにそのまま反映する。
- 分類が `unknown` の行は、対応する finding があれば ID(`G-*`)をテスト名または対象の列に併記する。
- gap 指摘表の「対象」は `target.kind` に応じて記入する。`subject` のときは `target.subject` の
  観測面名を、`entry` のときは `target.entries` の `T-*` を記入する。
- gap 指摘表の「推奨対応」には `findings[].suggestion` の内容を記入したうえで、この Skill では
  対応を実施しないことをテンプレート直後に再掲する。
- テスト一覧・gap 指摘のいずれも、該当する行がない場合は表を空のまま出さず `該当なし` と明記する。
