# Test Inventory Data 正規スキーマ

`test-audit` Skill の出力である Test Inventory Data の正規スキーマ(正本)を定義する。

## 目次

- 設計方針
- スキーマ本体
- blocking violation code
- 拡張余地(planned モード)

## 設計方針

- `inventory`(走査から再現可能な事実)と `findings`(gap 観点カタログを適用した判断)を分離する。
  同じ配列に混在させると、事実の記録と判断の記録が同じ検査規則の対象になり、事実側の再走査だけで
  済む更新と判断側だけを見直す更新を区別できなくなるため。
- `validation.blocking` は Data 自己整合検査(id 重複、参照切れ、語彙違反など)の名前空間とし、
  `findings` の gap 指摘とは別に扱う。blocking は Data として不整合であることを表し、findings は
  Data として整合したうえでの品質判断であるため、同じ名前空間に混ぜると承認可否の判定が曖昧になる。
- `mode: inventory` と `origin: implemented` は固定値として運用し、`mode: test-design` と
  `origin: planned` を将来の「実装前テスト設計モード」向けに語彙として予約する。今回はどちらの
  予約語彙も実装しないため、指定された場合は無効値として扱う。
- 安定 ID(`T-*`, `G-*`)は Branch Plan の AC ID と同じ契約に従い、再走査で振り直さない。ID の
  安定性が崩れると、過去の報告や外部からの参照(issue コメントなど)が指す対象を追跡できなくなる
  ため。

## スキーマ本体

```yaml
status: complete | partial | blocked
# complete: 走査対象を全て読解し、unscanned が空
# partial:  unscanned が非空。読めなかった範囲がある
# blocked:  validation.blocking が非空。Data として自己整合しない

mode: inventory                # test-design を将来予約する語彙。今回は inventory 固定
scope:
  requested: <ユーザー入力の走査対象。repository 全体 / 指定ディレクトリ / 指定ファイル>
  included_paths: []           # 実際に走査対象へ含めた path
  excluded_paths: []           # 明示的に対象外にした path

unscanned:                     # partial の根拠。読めなかった範囲を黙って落とさない
  - path: <path>
    reason: <権限不足 / バイナリ / 破損 / スコープ外判断など、読めなかった理由>

inventory:
  - id: T-1                    # 安定 ID。再走査で振り直さない
    name: <テスト名の原文>       # テストコード上の名前をそのまま転記する。言い換え禁止
    location: <path:line>      # テスト定義の位置
    origin: implemented        # planned は将来モードの予約語彙。今回は implemented 固定
    purpose: <このテストが存在する目的の1行要約>
    observed_behavior: <検証している観測可能な振る舞い。読解しても特定できないなら null>
    category: normal | boundary | error | unknown
    # normal:   正常系の代表値を検証する
    # boundary: 境界値(範囲の端、空、上限/下限など)を検証する
    # error:    異常系・例外経路を検証する
    # unknown:  上記いずれにも分類できない。それ自体が finding 候補になる
    subject: <被テスト対象の観測面。同じ観測面を検証する複数テストの集計キーにする>

findings:
  - id: G-1                    # 安定 ID。再走査で振り直さない
    code: <gap-catalog.md が定義する安定 code>
    target:
      kind: subject | entry    # subject: 対象観測面全体への指摘。entry: 個別テストへの指摘
      subject: <kind: subject のときの対象観測面。kind: entry のときは null>
      entries: []               # kind: entry のときの対象 T-*。kind: subject のときは空配列
    summary: <指摘内容の1行要約>
    evidence: <inventory のどの事実(id・category・subject の組み合わせ)から判定したか>
    suggestion: <推奨対応の説明。この Skill では実施せず、記述に留める>

validation:
  blocking: []                 # violation の配列。1件でもあれば status: blocked
  # - code: <blocking violation code 表の安定 code>
  #   path: <問題があるスキーマ上の path。例: inventory[2].id>
  #   message: <修正に必要な説明>
```

## blocking violation code

| code | 検査内容 |
| --- | --- |
| `duplicate-id` | `inventory[].id`(`T-*`)または `findings[].id`(`G-*`)の重複 |
| `unknown-reference` | `findings[].target.entries` が `inventory` に存在しない `T-*` を参照している |
| `vocabulary-invalid` | `mode`、`origin`、`category`、`target.kind` などスキーマが定めた語彙以外の値(`mode: test-design`、`origin: planned` を含む) |
| `state-invalid` | `status` と `unscanned` / `validation.blocking` の矛盾(例: `blocked` なのに `blocking` が空、`complete` なのに `unscanned` が非空) |
| `scope-violation` | `inventory[].location` が `scope.included_paths` / `scope.excluded_paths` と矛盾する(対象外パスからの inventory 混入など) |
| `finding-code-unknown` | `findings[].code` が [gap 観点カタログ](gap-catalog.md) に定義されていない code |

## 拡張余地(planned モード)

`mode: test-design` と `origin: planned` は、将来「実装前テスト設計モード」(Canon TDD の test
list に相当する、実装前にテスト項目だけを洗い出すモード)を同じスキーマへ追加する余地として語彙を
予約している。今回はこのモードを実装しない。

`mode` に `test-design` を、`origin` に `planned` を指定しても、この version のスキーマはこれを
有効な語彙として扱わない。指定された場合は `vocabulary-invalid` の blocking violation として
検出し、`status: blocked` にする。
