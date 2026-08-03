# Gunte

軍手 — 継ぎ手（[tugite](https://github.com/akitanabe/tugite)）を守る道具。

単一の正本集合から、指定された target（Claude Code / Codex 等）向けの文書を決定論的に出力する、prompt artifact のための契約検査つきコンパイラ。

> 同一の semantic build input から、バイト単位で同一の artifact 集合を生成する。
> v1 が保証するのはこの決定論と、生成時の契約検査（requires / forbids / order）だけである。

- 正本仕様: [#1 Gunte v1 正本 SPEC](https://github.com/akitanabe/gunte/issues/1)（Spec-Version 1 / Doc-Rev 10。正本は issue で管理し、リポジトリには commit しない）
