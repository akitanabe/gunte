# Gunte SPEC — Spec-Version 2

Status: normative
Date: 2026-08-09

Spec-Version 2はSpec-Version 1の生成、projection、serialization、requires/forbids/order、emit/checkの意味論を包含する。spec_version = 1の入力とartifact bytesは変更しない。v2専用fieldをv1で使うとunknown-key errorとする。

## V2-00 contract registry selection

v2 `gunte.toml`は任意の`[contracts].files`を持つ。`[contracts]`または`files`省略時は`["contracts.toml"]`、明示時は非空・stringだけ・project相対path・重複なしの配列で、既定fileを暗黙追加しない。glob、directory走査、include、remote、optional selectionは行わない。

app境界はselected fileを宣言順に全て読み、その`path + bytes` Dataをpure registry calculationへ渡す。registry順はselected file宣言順、そのfile内predicate宣言順である。header、inline table、dotted key、quoted keyの既存TOML形式を維持する。同一IDは全fileを通じた最初のpredicate位置を保持し、後のconcrete redefinitionをprimary、最初の位置をrelatedとする。TOMLのimplicit table introduction後の最初のexplicit table化はredefinitionではない。値のparseとschema validationはTOML parserの結果を正本とする。

selected fileのread failureはそのpathへ着地する。TOML syntax、schema、predicate validation、contract violationは該当fileのpredicateまたはfield位置へ着地する。全selected fileのread、parse、merge、validationを完了するまでartifact writerとatomic lock writerへ到達しない。

## CLI

CLIのroot Usageは`gunte <command> [options]`である。`emit`、`check`、`lock`、`help`をcommandとして持ち、rootでは`-h`と`--help`、各commandでは`--help`をhelp表示に使える。`gunte --help`、`gunte -h`、`gunte help`はroot helpを、`gunte help emit|check|lock`と`gunte emit|check|lock --help`はcommand helpを標準出力へ表示し、終了コード`0`を返す。helpはcwd取得、project execution、project file read、writer呼出しより前に完結するため、project外でも成功する。

root helpはGunteの目的、`gunte <command> [options]`、各commandの一行説明、`-h, --help`、cwd=project root、終了コード`0/1/2`、command help導線を示す。command helpはUsage、目的、主要input、書込/副作用、option、成功/失敗条件、例を示す。`emit`は全input/source/generation/contract/registry検証後にwriteする。`check`はread-onlyでartifact bytes、v2 managed inventory、v2 full lock mismatchを検査する。`lock`はv2専用でtarget optionを持たず、全検証後に`gunte.lock.json`を更新する。`emit`と`check`のtarget optionは`--target ID`で、project全体を検証しtarget固有の処理だけをIDで限定する。contract registry inputはv2では`gunte.toml`の`[contracts].files`で選択された全fileである。

unknown command、不正option、不完全なtarget、unknownまたは余分なhelp引数は標準エラー出力へ次の既存usage messageを表示し、終了コード`2`を返す。引数なしも同じである。

```text
usage: gunte emit|check [--target ID] | gunte lock
```

## V2-01 version single source

v2 [project] は version またはproject相対path version_from のexactly oneを必須とする。version fileはUTF-8で、先頭BOMを最大1個除去し、CRLF/bare CRをLFへ正規化する。その後、末尾LFが0個ならそのまま、1個なら1個だけ除去して、残りが非空かつLFを含まないことを要求する。末尾LFが2個以上、または本文途中LFは単一行違反である。前後空白はopaque valueの一部として保持する。version fileはsemantic input、source inventory期待file、collision、lockの対象である。

## V2-02 managed inventory

v2 [sources] と各 [targets.<id>] は managed_roots, allow_files, allow_dirs を持てる。source値はproject相対、target値はoutput_root相対である。未指定は空で所有しない。展開後managed rootsはsource/全target間で同一・ancestor/descendant overlapを禁止する。allow entryはexactly one root配下で、allow同士の冗長overlapを禁止する。

checkはmanaged rootだけをread-only再帰走査する。source期待fileはselected contract files、version_from、sources.files、target期待fileは今回算出artifactである。期待/allow/rootのancestor directory、allow_file exact entry、allow_dirと全descendantは正当で、それ以外のfile/directoryをinventory mismatchとする。存在しないallowは許容する。symlink directoryは追跡せずleafとして扱い、期待file/allow_fileだけに一致できる。managed scope外をstaleにしない。全target ruleに不一致のsourceもcheck failure。通常emitはstaleを削除せず、clean commandはv2にない。

## V2-03 typed structural contract

v2 [contracts.<id>] kind = "structure" は subject, paths, assertions を持つ。subjectはsource_frontmatterまたはartifact。pathsはproject相対anchored patternで、*はslashを跨がず**はない。source_frontmatterはTOMLでformat/applies_toを持たない。artifactはformat = "yaml" | "toml" | "json" と非空既知targetのapplies_toを必須とし、yamlはmarkdown+yaml-frontmatter-v1の厳密な先頭frontmatter blockまたはmarkdown-v1の全artifact bytes、toml/jsonは各々toml-v1/json-v1 profileだけに適用する。selector 0 matchはV2-05の対象scopeのfailure、複数pattern一致documentは一度だけ評価する。

assertion pathは空文字のdocument rootまたはdot segmentで、*はmappingの保持された宣言順またはlist index順に展開し、順序付きnode列を返す。existsは件数1以上、absentは0、cardinalityはcount完全一致。equalsはexactly one nodeとtyped deep equality、exact_keysはexactly one mappingと重複なしstring valueのkey set一致、list_orderはexactly one listとtyped array順序一致、list_setはexactly one scalar-only listでactual/expected双方重複なしのtyped set一致。operand欠落/余分/type不正はconfig error、その他のtruth failureはcontract failure。各selected documentに全assertionを適用する。

source frontmatter parserはv1 adapter用mapに加えてnested mappingの宣言順を持つordered node Dataを生成し、structure evaluatorだけがordered nodeを使う。artifact YAMLはmarkdown+yaml-frontmatter-v1なら先頭frontmatterのopening ---直後から対応する最初のclosing ---直前だけを、markdown-v1なら全bytesを、各々exactly one YAML documentとしてstrict Node decodeする。mapping duplicate keyはlast-win前に失敗する。TOML/JSONも各strict parserのduplicate拒否を維持する。typed calculationはI/Oを行わない。

## V2-04 registry integrity and lock

v2では全@contract spanをslice付きrequires/forbidsがsyntactically参照し、全anchorをorderがsyntactically参照しなければならない。target別emission/resolvability/truthはV2-05に従う。

slice付きrequires/forbids IDは<human-prefix>-<hash12>。hashはvalidated Dataの固定順compact canonical JSON+LFのSHA-256 lowercase先頭12で、key順はkind,slice,pattern,applies_to、applies_to宣言順を保持する。

gunte.lock.jsonは2-space canonical JSON+LFで、key順はspec_version,semantic_inputs,contracts,declarations。semantic_inputsはgunte.toml,selected contract files,version_from,sources.filesをこの順で最初の出現だけ採用する。contractsはmerged registry宣言順のid,sha256、declarationsはsource/IR宣言順のkind,id,path。contract sha256はraw TOMLでなくvalidated Dataのcompact canonical JSON+LFから算出する。text key順はtype,id,kind,slice,pattern,before,after,applies_to。structure key順はtype,id,subject,paths,format,applies_to,assertions。assertion key順はpath,op,value,count。textのslice,pattern,before,afterは欠落時null。structureのformatは欠落時null、source_frontmatterのapplies_toは[]。assertionのvalue,countは欠落時null。typed valueはstring,int64,bool,list,mapだけで、map keyはUTF-8 byte lexicographic、list/paths/assertions/applies_toは宣言順である。

canonical JSON stringはquoteとbackslashをescapeし、U+0008/U+0009/U+000A/U+000C/U+000Dを短縮escape、その他U+0000–U+001FとU+007Fをlowercase \u00xxでescapeする。<, >, &, U+2028, U+2029、その他非ASCIIはescapeしない。file末尾LFは1個である。preimage fixtureは<, >, &, 非ASCII、control、optional fieldを含む。

projectを実行するcommandはgunte emit|check [--target ID]とv2専用gunte lockである。lockはtarget optionを持たず、既存lockなしでbootstrapできる。全input/source/generation/contract/registry検証後に同一directory temp write・file sync・close・renameを行い、成功後bytesを再照合する。rename前の失敗ではtemp cleanupを試み旧lock bytesを保持する。rename結果不明ではold/new/otherを再観測しblind retryしない。emitはlockを更新しない。v2 checkはfull lock欠落/差を失敗する。lockは意味品質や変更承認を行わない。process crash durability、directory fsync、別processとの同時実行、全platformでのatomic replaceはv2保証外である。

## V2-05 target scope

--target Aでもfull projectでconfig/contracts parse、version、全source parse/ID、source inventory、全target不一致source、source_frontmatter structure、predicate hash、syntactic unused declaration、full lockを検査する。Aだけでartifact生成、artifact structure selector/decode/assertion、target別reference emission/resolvability、text predicate truth、output inventory、artifact byte compareを検査する。target省略時は全target。v1の既存target selectionは変更しない。

## V2-06 diagnostics and compatibility

checkはwriterを呼ばず常にread-only。診断primaryは対象pathまたはconfig/contract宣言位置で、必要なら関連する最初のpredicate位置、source、artifactを付ける。Spec-Version 1のoracle、CLI、bytes、target selectionは不変。Spec-Version 2のmanaged scope外pathは所有せず、通常emitはstaleを削除せず、lock更新は契約変更の自動承認ではない。
