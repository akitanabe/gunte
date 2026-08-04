# Gunte アンチパターン集

## 散文依存の契約テスト（Prose-Coupled Contract Tests）

Prompt artifact の契約を、本文中の文字列、見出し、箇条書き、手順番号などの**現在の文章表現と配置**から逆算して検査する設計。

守りたいのは「この命題がこの規範を担う」「この手順は別の手順より前に現れる」といった仕様上の不変条件である。しかし、その不変条件を source 側に宣言せず、テストが Markdown の見た目から推測すると、文章編集と契約変更を区別できなくなる。

`assertIn` / `assertNotIn` だけで始められるため、最初は安価に見える。規模が大きくなると、テスト補助関数が手書きの Markdown parser へ成長し、保証を強めるよりも変更とレビューの負債を増やす。

---

## Tugite で起きたこと

Tugite では、prompt 原稿中の日本語文字列や配置を直接検査する契約テストが増加した。Issue 作成時点では次の規模に達していた。

| 指標 | 規模 |
|---|---:|
| 契約テスト | 287 method |
| 日本語を含む文字列 literal | 約 2,600 |

`feature-lead` の Branch Plan 単位授権を変更した事例では、レビューが14 round・38指摘に達した。Acceptance Criteria の実装そのものへの指摘は12件で、残る26件はテストの scope、helper の波及、コメントの正確さなど、テスト機構の境界に関するものだった。前 round の修正が次の問題を生む連鎖も5回発生した。

問題の中心は、原稿側に「どの範囲がどの契約を担うか」を示す安定した識別子がなかったことにある。テストは代わりに、見出し、bullet marker、手順番号、substring などから意味上の帰属を推測した。

たとえば、`delegation` を設定してから `impl-lead` へ渡す順序は、逆転すると境界で無限ループし得る重要な契約である。しかし source 上で宣言されていなかったため、テストは「手順6」「手順7」という配置から意味を復元しようとした。

その結果、`_section`、`_bullet`、`_numbered_step` などの helper が育った。これらは契約を検査する helper ではなく、散文から失われた意味構造を復元する小さな Markdown parser だった。

### 関連 Issue

| Issue | 役割 |
|---|---|
| [Tugite #133](https://github.com/akitanabe/tugite/issues/133) | 失敗の分析と、仕様上の不変条件をテストするルールの確立 |
| [Tugite #134](https://github.com/akitanabe/tugite/issues/134) | 契約の範囲と順序を宣言して生成器で検査する設計 |
| [Tugite #135](https://github.com/akitanabe/tugite/issues/135) | 既存テストを分類し、構造化・削除・eval移行を判断する方針 |

---

## 何が問題になるか

### 文章編集と契約変更を区別できない

見出し名、説明順、bullet の位置、言い換えを固定すると、意味を変えていない編集でも失敗する。逆に、同じ文字列が別の場所に残っていれば、契約が壊れていてもテストが通る。

### テストが独自 parser になる

section、bullet、numbered step の抽出が増えると、テスト側に文法・責務・互換性範囲の不明確な parser が生まれる。helper の変更は多数のテストへ波及する。

### 壊れる方向が green になる

次の誤りは通常のテスト実行で失敗せず、むしろ green になりやすい。

- scope が広すぎて別の場所の文字列を拾う
- slice が空になり、禁止語検査が何も見ない
- substring 検索が囮の文字列へ一致する
- 順序ではなく手順番号の存在だけを検査する

この状態では「テストが通る」が保証の証拠にならない。重要な知識は再実行可能な test suite ではなく、レビュー履歴へ蓄積してしまう。

### 移行先がなければ削除できない

散文依存テストが問題でも、重要な契約を表現する代替手段がなければ削除できない。先に契約の構造化手段を用意し、その後で既存テストを分類して移行する必要がある。

---

## 典型例

次は Tugite の実コードではなく、発生した構造を単純化した概念例である。

```python
section = _section(document, "全体の流れ")
step6 = _numbered_step(section, 6)
step7 = _numbered_step(section, 7)

self.assertIn("delegation", step6)
self.assertIn("impl-lead", step7)
self.assertNotIn("implementation_stages", section)
```

このテストからは、抽出範囲が正しいか、順序そのものを検査しているか、別の文脈にある文字列へ偶然一致していないかを判別できない。

---

## Gunte による置き換え

Gunte v1 では、「判断ではなく宣言へ移す」という方向を次の要素で表現する。

- source の `@contract` span: 契約を検査する範囲
- source の `@anchor`: 順序上の位置
- `contracts.toml` の named predicate: `requires` / `forbids` / `order`
- `gunte check`: semantic build input から生成される artifact との byte drift 検出

### Source

```markdown
<!-- @anchor delegation-ready -->
Branch Plan の `delegation` を確定する。

<!-- @anchor handoff-start -->
<!-- @contract handoff-rule -->
確定した Branch Plan を `impl-lead` へ渡す。
<!-- @/contract -->
```

### `contracts.toml`

```toml
[contracts.handoff-target]
kind = "requires"
slice = "handoff-rule"
pattern = "impl-lead"
applies_to = ["claude", "codex"]

[contracts.delegation-before-handoff]
kind = "order"
before = "delegation-ready"
after = "handoff-start"
applies_to = ["claude", "codex"]

[contracts.no-legacy-implementation-stages]
kind = "forbids"
pattern = "implementation_stages"
applies_to = ["claude", "codex"]
```

この形では、検査範囲と意味を持つ順序が source に明示され、廃止語は生成後 artifact に対する禁止条件になる。文章を言い換えても predicate を満たす限り、宣言された機械的契約は維持される。

---

## Gunte を使っても避けるべきこと

- **すべての文章へ contract を付ける:** 変更時に明示的な仕様判断を要求したい不変条件だけを契約にする。
- **意味品質を substring で証明する:** LLMの判断品質、妥当性、収束性などは eval またはレビューで扱う。
- **読みやすさのための順序を固定する:** `order` は、逆転すると実行や解釈が変わる順序に限定する。
- **生成物へ文字列テストを重ねる:** Gunte が検査する投影、置換、serialization、predicate、drift を通常の unit test で二重に固定しない。

## 検査手段の選び分け

| 守りたいもの | 適切な手段 |
|---|---|
| 決定論的な artifact 生成とdrift | Gunte `emit` / `check` |
| 特定範囲の必須表現 | `requires` |
| 廃止語・禁止表現 | `forbids` |
| 実行・解釈上の意味を持つ順序 | `order` + `@anchor` |
| 必須ファイルやmetadataなどの構造 | Gunte設定またはproject固有のstructural test |
| 判断品質、妥当性、収束性 | eval |
| 読みやすさ、表現品質 | editorial review |

---

## 既存 repository からの移行

1. **テストを目的で分類する。** 構造検査は維持し、重要な workflow 契約は構造化し、表現固定は削除し、意味品質は eval へ移す。
2. **先に移行先を作る。** 重要契約の span / anchor / predicate を追加してから旧テストを削除し、無保護期間を作らない。
3. **契約の境界ごとに移行する。** 関連する source、skill、artifact 集合などの小さな単位で進める。
4. **変異で確認する。** 必須表現の範囲外移動、禁止語の残存、anchor の逆転、target固有の欠落を入れ、新しい契約が失敗することを確認する。

移行対象を見つける兆候は、長い自然言語 literal、大量の見出し・手順番号依存、Markdown抽出 helper、空または過大な scope、生成物ごとの重複した文字列検査である。

---

## 原則

> テストは、現在の文章表現ではなく仕様上の不変条件を守る。

> 契約の範囲と順序をテスト側で推測しない。source 側で宣言する。

> 宣言できない意味品質は、文字列テストで近似せず eval またはレビューへ送る。

Gunte は文章の意味そのものを証明しない。単一の semantic build input、明示された span / anchor、名前付き predicate、決定論的な artifact 生成という、再実行可能な参照先を提供する。

---

## 出典

- [Tugite #133: テスト作成ルールを定め散文依存の契約テストを禁止する](https://github.com/akitanabe/tugite/issues/133)
- [Tugite #134: 契約IDを導入し原稿の契約を生成器で検査する](https://github.com/akitanabe/tugite/issues/134)
- [Tugite #135: 既存契約テストを棚卸しし散文依存の検査を減らす](https://github.com/akitanabe/tugite/issues/135)
- [Gunte README](https://github.com/akitanabe/gunte)
