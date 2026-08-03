<!-- Generated from shared/. Do not edit directly. -->

# Expert 選択

`expert-implementer` は高コストな例外的 worker である。公開 API、data migration、security、concurrency、
変更行数、重要度などの属性だけでは選ばない。通常は `senior-implementer` を使い、親相当の能力が必要で
senior では不足する具体的根拠がある独立実装枝だけを候補にする。

起動前に次の Data を `expert-selection-reviewer` へ渡す。

- タスクと受け入れ条件
- 確定済みの scope
- 親相当の能力が必要な判断
- senior では不足すると判断した具体的根拠
- 独立 context へ隔離する理由
- 副作用と責務境界に関する制約
- expert を使わない場合に予想される再設計・再実装

`expert-selection-reviewer` は選択理由だけを審査し、仕様補完、実装設計、タスク分割、worker 起動、
最終判断を行わない。判定は次のいずれかに固定する。

- `APPROVE_EXPERT`
- `REJECT_USE_SENIOR`
- `REJECT_USE_IMPLEMENTER`
- `REJECT_REPLAN`

`APPROVE_EXPERT` の場合だけ expert を起動する。reject の場合は、その委譲フロー内で expert を起動せず、
提案された worker へ自動 fallback しない。親がタスク分割、AC、worker 選択、直接実装を含むプランを
練り直す。判定に同意できない場合も reviewer を無視せず、選択理由またはプランを変更して再審査する。

expert の委譲 prompt には次を含める。

```text
Expert 選択理由:
- 親相当の能力が必要な判断:
- senior では不足すると判断した根拠:
- 独立 context へ隔離する理由:

Expert 選択審査:
- 判定: APPROVE_EXPERT
- 承認理由:
```

外部 I/O、共有可変状態、現在時刻、乱数などの副作用があるだけでは expert の選択理由にならない。
副作用の局所化、整合性、部分失敗、再試行、冪等性、transaction 境界が複数の責務境界にまたがり、
親相当の判断が必要であることまで具体化する。

`gpt-5.6-sol`、指定 reasoning effort、custom agent、`expert-selection-reviewer`、または起動機構を利用できず、
事前審査または expert の起動を完了できない場合は現在の委譲フローを終了する。

利用不能の内容と未着手・未完了範囲を報告し、senior への自動 fallback や親による直接実装を続行しない。
タスク分割、worker 選択、直接実装を含むプランの練り直しは、改めて開始する。
