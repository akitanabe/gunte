# Gunte SPEC — Spec-Version 2

Status: normative
Date: 2026-08-09

Spec-Version 2はSpec-Version 1の生成、projection、serialization、requires/forbids/order、emit/checkの意味論を包含する。spec_version = 1の入力とartifact bytesは変更しない。v2専用fieldをv1で使うとunknown-key errorとする。

## V2-01 version single source

v2 [project] は version またはproject相対path version_from のexactly oneを必須とする。version fileはUTF-8で、先頭BOMを最大1個除去し、CRLF/bare CRをLFへ正規化する。その後、末尾LFが0個ならそのまま、1個なら1個だけ除去して、残りが非空かつLFを含まないことを要求する。末尾LFが2個以上、または本文途中LFは単一行違反である。前後空白はopaque valueの一部として保持する。version fileはsemantic input、source inventory期待file、collision、lockの対象である。

## V2-02 managed inventory

v2 [sources] と各 [targets.<id>] は managed_roots, allow_files, allow_dirs を持てる。source値はproject相対、target値はoutput_root相対である。未指定は空で所有しない。展開後managed rootsはsource/全target間で同一・ancestor/descendant overlapを禁止する。allow entryはexactly one root配下で、allow同士の冗長overlapを禁止する。

checkはmanaged rootだけをread-only再帰走査する。source期待fileはsources.filesとversion_from、target期待fileは今回算出artifactである。期待/allow/rootのancestor directory、allow_file exact entry、allow_dirと全descendantは正当で、それ以外のfile/directoryをinventory mismatchとする。存在しないallowは許容する。symlink directoryは追跡せずleafとして扱い、期待file/allow_fileだけに一致できる。managed scope外をstaleにしない。全target ruleに不一致のsourceもcheck failure。通常emitはstaleを削除せず、clean commandはv2にない。

## V2-03 typed structural contract

v2 [contracts.<id>] kind = "structure" は subject, paths, assertions を持つ。subjectはsource_frontmatterまたはartifact。pathsはproject相対anchored patternで、*はslashを跨がず**はない。source_frontmatterはTOMLでformat/applies_toを持たない。artifactはformat = "yaml" | "toml" | "json" と非空既知targetのapplies_toを必須とし、yaml/toml/jsonは各々markdown+yaml-frontmatter-v1/toml-v1/json-v1 profileだけに適用する。selector 0 matchはV2-05の対象scopeのfailure、複数pattern一致documentは一度だけ評価する。

assertion pathはdot segmentで、*はmappingの保持された宣言順またはlist index順に展開し、順序付きnode列を返す。existsは件数1以上、absentは0、cardinalityはcount完全一致。equalsはexactly one nodeとtyped deep equality、exact_keysはexactly one mappingと重複なしstring valueのkey set一致、list_orderはexactly one listとtyped array順序一致、list_setはexactly one scalar-only listでactual/expected双方重複なしのtyped set一致。operand欠落/余分/type不正はconfig error、その他のtruth failureはcontract failure。各selected documentに全assertionを適用する。

source frontmatter parserはv1 adapter用mapに加えてnested mappingの宣言順を持つordered node Dataを生成し、structure evaluatorだけがordered nodeを使う。artifact YAMLは先頭frontmatterのopening ---直後から対応する最初のclosing ---直前だけをstrict Node decodeし、後続Markdownを含めない。mapping duplicate keyはlast-win前に失敗する。TOML/JSONも各strict parserのduplicate拒否を維持する。typed calculationはI/Oを行わない。

## V2-04 registry integrity and lock

v2では全@contract spanをslice付きrequires/forbidsがsyntactically参照し、全anchorをorderがsyntactically参照しなければならない。target別emission/resolvability/truthはV2-05に従う。

slice付きrequires/forbids IDは<human-prefix>-<hash12>。hashはvalidated Dataの固定順compact canonical JSON+LFのSHA-256 lowercase先頭12で、key順はkind,slice,pattern,applies_to、applies_to宣言順を保持する。

gunte.lock.jsonは2-space canonical JSON+LFで、key順はspec_version,semantic_inputs,contracts,declarations。semantic_inputsはgunte.toml,contracts.toml,version_from,sources.filesをこの順で重複排除。contractsは宣言順のid,sha256、declarationsはsource/IR宣言順のkind,id,path。contract sha256はraw TOMLでなくvalidated Dataのcompact canonical JSON+LFから算出する。text key順はtype,id,kind,slice,pattern,before,after,applies_to。structure key順はtype,id,subject,paths,format,applies_to,assertions。assertion key順はpath,op,value,count。textのslice,pattern,before,afterは欠落時null。structureのformatは欠落時null、source_frontmatterのapplies_toは[]。assertionのvalue,countは欠落時null。typed valueはstring,int64,bool,list,mapだけで、map keyはUTF-8 byte lexicographic、list/paths/assertions/applies_toは宣言順である。

canonical JSON stringはquoteとbackslashをescapeし、U+0008/U+0009/U+000A/U+000C/U+000Dを短縮escape、その他U+0000–U+001FとU+007Fをlowercase \u00xxでescapeする。<, >, &, U+2028, U+2029、その他非ASCIIはescapeしない。file末尾LFは1個である。preimage fixtureは<, >, &, 非ASCII、control、optional fieldを含む。

CLIはgunte emit|check [--target ID]とv2専用gunte lock。lockはtarget optionを持たず、既存lockなしでbootstrapできる。全input/source/generation/contract/registry検証後に同一directory temp write・file sync・close・renameを行い、成功後bytesを再照合する。rename前の失敗ではtemp cleanupを試み旧lock bytesを保持する。rename結果不明ではold/new/otherを再観測しblind retryしない。emitはlockを更新しない。v2 checkはfull lock欠落/差を失敗する。lockは意味品質や変更承認を行わない。process crash durability、directory fsync、別processとの同時実行、全platformでのatomic replaceはv2保証外である。

## V2-05 target scope

--target Aでもfull projectでconfig/contracts parse、version、全source parse/ID、source inventory、全target不一致source、source_frontmatter structure、predicate hash、syntactic unused declaration、full lockを検査する。Aだけでartifact生成、artifact structure selector/decode/assertion、target別reference emission/resolvability、text predicate truth、output inventory、artifact byte compareを検査する。target省略時は全target。v1の既存target selectionは変更しない。

## V2-06 diagnostics and compatibility

checkはwriterを呼ばず常にread-only。診断primaryは対象pathまたはconfig/contract宣言位置で、必要なら関連source/artifactを付ける。Spec-Version 1のoracle、CLI、bytes、target selectionは不変。Spec-Version 2のmanaged scope外pathは所有せず、通常emitはstaleを削除せず、lock更新は契約変更の自動承認ではない。
