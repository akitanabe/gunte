# gap 観点カタログ

Test Inventory Data の `findings[].code` に使う安定 code の正本を定義する。code は `gap-*` と
`test-*` の2系統に分かれる。`gap-*` は被テスト対象の観測面(`subject`)単位の不足、`test-*` は
個別テスト(`entry`)単位の問題を指す。判定はテストスイート全体を対象とし、Acceptance Criteria や
diff の有無に依存しない。

## 目次

- gap-*(subject 単位の不足)
- test-*(entry 単位の問題)
- 既存 reviewer との境界

## gap-*(subject 単位の不足)

### gap-equivalence-missing

- 観点: 同値分割。対象観測面の入力クラスのうち、代表値を検証するテストが欠けている。
- target.kind: `subject`
- evidence: 対象 `subject` に属する `inventory` を横断し、実装または仕様から識別できる入力クラスと、
  各クラスを検証しているテストの対応を示す。
- 指摘しない条件: 入力クラスが単一で分割の余地がない場合、または対象 `subject` にテストが1件も
  なく `gap-untested-subject` に既に含まれる場合は指摘しない。実装コードを読んで新たな入力クラスの
  存在を推測し、それだけを根拠に指摘しない。

### gap-boundary-missing

- 観点: 境界値分析。対象観測面が持つ範囲の境界(上限・下限・空・ゼロ件・最大長など)を検証する
  テストが欠けている。
- target.kind: `subject`
- evidence: 対象 `subject` の `category: boundary` のテストが inventory 上に存在するかを確認し、
  存在しない、または一部の境界だけを検証していることを示す。
- 指摘しない条件: 対象観測面に境界という概念が存在しない(離散的な有限選択肢のみなど)場合は
  指摘しない。境界の候補を実装コードから新たに読み取って仕様を補完し、それを根拠に指摘しない。

### gap-error-path-missing

- 観点: 異常系網羅。対象観測面の異常入力・例外経路・失敗時の振る舞いを検証するテストが欠けている。
- target.kind: `subject`
- evidence: 対象 `subject` の `category: error` のテストが inventory 上に存在するかを確認し、
  存在しないことを示す。
- 指摘しない条件: 対象観測面が失敗しうる経路を持たない(純粋な参照・定数返却など)場合は指摘しない。

### gap-untested-subject

- 観点: 網羅性の前提。対象観測面にテストが1件も存在しない。
- target.kind: `subject`
- evidence: 走査で識別した `subject` のうち、`inventory` 中に同じ `subject` を持つエントリが
  0件であることを示す。
- 指摘しない条件: 走査範囲外の `subject`(`scope.excluded_paths` に属する)には指摘しない。この
  code が成立する `subject` には `gap-equivalence-missing` 等の他 `gap-*` を重ねて指摘しない
  (対象が未テストであること自体が根本原因であり、個別観点の欠落は派生情報にならないため)。

## test-*(entry 単位の問題)

### test-implementation-coupled

- 観点: 実装詳細への依存。テストが外部から観測可能な振る舞いではなく、内部構造や実装手順に依存
  した期待値を持つ。
- target.kind: `entry`
- evidence: 対象テスト(`T-*`)のアサーションや準備処理が、private メソッド呼び出し、内部状態の
  直接参照、実装から逆算した期待値のいずれかに依存していることを、テストコードの記述から示す。
- 指摘しない条件: 契約として公開されている内部 API(明示的に公開された helper など)への依存は
  指摘しない。実装を読んでリファクタリング可能性を推測し、それだけを根拠に指摘しない。

### test-name-unreadable

- 観点: What の可読性。テスト名から検証している振る舞いを読み取れない。
- target.kind: `entry`
- evidence: 対象テスト(`T-*`)の名前が、動作条件と期待結果のいずれか、または両方を欠いていることを
  名前の記述から示す。
- 指摘しない条件: プロジェクトの命名規約上、名前以外(記述ブロックのネストなど)で条件・結果を
  表現できている場合は指摘しない。

### test-behavior-unclear

- 観点: What の特定可能性。テストが検証している観測可能な振る舞いを、テストコードの読解だけからは
  特定できない。
- target.kind: `entry`
- evidence: 対象テスト(`T-*`)の `observed_behavior` が `null` になった根拠(準備処理・アサーション
  ・名前のいずれからも意図を読み取れないこと)を示す。
- 指摘しない条件: 読解に時間をかければ特定できる程度の複雑さは指摘しない。この code は
  `observed_behavior: null` になった場合にのみ成立する。

## 既存 reviewer との境界

- `test-quality-reviewer`: 観点語彙(観測可能な振る舞い、境界値、異常系)を共有するが、あちらは
  diff スコープと Acceptance Criteria 根拠を前提にした受け入れ gate 判定である。本 Skill は
  テストスイート全体を対象にし、AC も gate も持たない。AC 対応、Red 証跡、mock による隠蔽、既存
  テストの弱体化は AC と diff がないと判定できないため、本 Skill の観点には含めない。
- `over-engineering-reviewer`: 基準 commit からの diff が導入した要素を削減方向で検出する
  reviewer である。本 Skill はテストの過剰・重複を削減方向で指摘する観点を持たない。
- `writing-principles-reviewer`: `test-name-unreadable` は同じ What の観点を扱うが、本 Skill では
  指摘 Data として `findings` に記録するだけであり、記述原則の gate 判定を代替しない。
